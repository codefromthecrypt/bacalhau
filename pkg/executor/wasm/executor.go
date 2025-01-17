package wasm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/bacalhau-project/bacalhau/pkg/bidstrategy"
	"github.com/bacalhau-project/bacalhau/pkg/executor"
	"github.com/bacalhau-project/bacalhau/pkg/model"
	"github.com/bacalhau-project/bacalhau/pkg/storage"
	"github.com/bacalhau-project/bacalhau/pkg/storage/util"
	"github.com/bacalhau-project/bacalhau/pkg/system"
	"github.com/bacalhau-project/bacalhau/pkg/util/closer"
	"github.com/bacalhau-project/bacalhau/pkg/util/filefs"
	"github.com/bacalhau-project/bacalhau/pkg/util/mountfs"
	"github.com/bacalhau-project/bacalhau/pkg/util/touchfs"
	"github.com/c2h5oh/datasize"
	"github.com/rs/zerolog/log"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"golang.org/x/exp/maps"
)

type Executor struct {
	StorageProvider storage.StorageProvider
}

func NewExecutor(_ context.Context, storageProvider storage.StorageProvider) (*Executor, error) {
	return &Executor{
		StorageProvider: storageProvider,
	}, nil
}

func (e *Executor) IsInstalled(context.Context) (bool, error) {
	// WASM executor runs natively in Go and so is always available
	return true, nil
}

func (e *Executor) HasStorageLocally(ctx context.Context, volume model.StorageSpec) (bool, error) {
	ctx, span := system.NewSpan(ctx, system.GetTracer(), "pkg/executor/wasm.Executor.HasStorageLocally")
	defer span.End()

	s, err := e.StorageProvider.Get(ctx, volume.StorageSource)
	if err != nil {
		return false, err
	}

	return s.HasStorageLocally(ctx, volume)
}

func (e *Executor) GetVolumeSize(ctx context.Context, volume model.StorageSpec) (uint64, error) {
	ctx, span := system.NewSpan(ctx, system.GetTracer(), "pkg/executor/wasm.Executor.GetVolumeSize")
	defer span.End()

	storageProvider, err := e.StorageProvider.Get(ctx, volume.StorageSource)
	if err != nil {
		return 0, err
	}
	return storageProvider.GetVolumeSize(ctx, volume)
}

// GetBidStrategy implements executor.Executor
func (*Executor) GetBidStrategy(context.Context) (bidstrategy.BidStrategy, error) {
	return bidstrategy.NewChainedBidStrategy(), nil
}

// makeFsFromStorage sets up a virtual filesystem (represented by an fs.FS) that
// will be the filesystem exposed to our WASM. The strategy for this is to:
//
//   - mount each input at the name specified by Path
//   - make a directory in the job results directory for each output and mount that
//     at the name specified by Name
func (e *Executor) makeFsFromStorage(ctx context.Context, jobResultsDir string, inputs, outputs []model.StorageSpec) (fs.FS, error) {
	var err error
	rootFs := mountfs.New()

	volumes, err := storage.ParallelPrepareStorage(ctx, e.StorageProvider, inputs)
	if err != nil {
		return nil, err
	}

	for input, volume := range volumes {
		log.Ctx(ctx).Debug().
			Str("input", input.Path).
			Str("source", volume.Source).
			Msg("Using input")

		var stat os.FileInfo
		stat, err = os.Stat(volume.Source)
		if err != nil {
			return nil, err
		}

		var inputFs fs.FS
		if stat.IsDir() {
			inputFs = os.DirFS(volume.Source)
		} else {
			inputFs = filefs.New(volume.Source)
		}

		err = rootFs.Mount(input.Path, inputFs)
		if err != nil {
			return nil, err
		}
	}

	for _, output := range outputs {
		if output.Name == "" {
			return nil, fmt.Errorf("output volume has no name: %+v", output)
		}

		if output.Path == "" {
			return nil, fmt.Errorf("output volume has no path: %+v", output)
		}

		srcd := filepath.Join(jobResultsDir, output.Name)
		log.Ctx(ctx).Debug().
			Str("output", output.Name).
			Str("dir", srcd).
			Msg("Collecting output")

		err = os.Mkdir(srcd, util.OS_ALL_R|util.OS_ALL_X|util.OS_USER_W)
		if err != nil {
			return nil, err
		}

		err = rootFs.Mount(output.Name, touchfs.New(srcd))
		if err != nil {
			return nil, err
		}
	}

	return rootFs, nil
}

//nolint:funlen  // Will be made shorter when we do more module linking
func (e *Executor) Run(ctx context.Context, job model.Job, jobResultsDir string) (*model.RunCommandResult, error) {
	ctx, span := system.NewSpan(ctx, system.GetTracer(), "pkg/executor/wasm.Executor.Run")
	defer span.End()

	engineConfig := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)

	// Apply memory limits to the runtime. We have to do this in multiples of
	// the WASM page size of 64kb, so round up to the nearest page size if the
	// limit is not specified as a multiple of that.
	if job.Spec.Resources.Memory != "" {
		memoryLimit, err := datasize.ParseString(job.Spec.Resources.Memory)
		if err != nil {
			return executor.FailResult(err)
		}

		const pageSize = 65536
		pageLimit := memoryLimit.Bytes()/pageSize + system.Min(memoryLimit.Bytes()%pageSize, 1)
		engineConfig = engineConfig.WithMemoryLimitPages(uint32(pageLimit))
	}

	engine := tracedRuntime{wazero.NewRuntimeWithConfig(ctx, engineConfig)}
	defer closer.ContextCloserWithLogOnError(ctx, "engine", engine)

	module, err := LoadRemoteModule(ctx, engine, e.StorageProvider, job.Spec.Wasm.EntryModule)
	if err != nil {
		return executor.FailResult(err)
	}

	rootFs, err := e.makeFsFromStorage(ctx, jobResultsDir, job.Spec.Inputs, job.Spec.Outputs)
	if err != nil {
		return executor.FailResult(err)
	}

	// Configure the modules. We will write STDOUT and STDERR to a buffer so
	// that we can later include them in the job results. We don't want to
	// execute any start functions automatically as we will do it manually
	// later. Finally, add the filesystem which contains our input and output.
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	args := append([]string{module.Name()}, job.Spec.Wasm.Parameters...)

	config := wazero.NewModuleConfig().
		WithStartFunctions().
		WithStdout(stdout).
		WithStderr(stderr).
		WithArgs(args...).
		WithFS(rootFs)

	keys := maps.Keys(job.Spec.Wasm.EnvironmentVariables)
	sort.Strings(keys)
	for _, key := range keys {
		// Make sure we add the environment variables in a consistent order
		config = config.WithEnv(key, job.Spec.Wasm.EnvironmentVariables[key])
	}

	// Load and instantiate imported modules
	var importedModules []wazero.CompiledModule
	for _, wasmSpec := range job.Spec.Wasm.ImportModules {
		importedWasi, err := LoadRemoteModule(ctx, engine, e.StorageProvider, wasmSpec)
		if err != nil {
			return executor.FailResult(err)
		}
		importedModules = append(importedModules, importedWasi)

		if _, err := engine.InstantiateModule(ctx, importedWasi, config); err != nil {
			return executor.FailResult(err)
		}
	}

	wasi, err := wasi_snapshot_preview1.NewBuilder(engine).Compile(ctx)
	if err != nil {
		return executor.FailResult(err)
	}

	if _, err := engine.InstantiateModule(ctx, wasi, config); err != nil {
		return executor.FailResult(err)
	}

	// Now instantiate the module and run the entry point.
	instance, err := engine.InstantiateModule(ctx, module, config)
	if err != nil {
		return executor.FailResult(err)
	}

	// Check that all WASI modules conform to our requirements.
	importedModules = append(importedModules, wasi)

	if err := ValidateModuleAgainstJob(module, job.Spec, importedModules...); err != nil {
		return executor.FailResult(err)
	}

	// The function should exit which results in a sys.ExitError. So we capture
	// the exit code for inclusion in the job output, and ignore the return code
	// from the function (most WASI compilers will not give one). Some compilers
	// though do not set an exit code, so we use a default of -1.
	log.Ctx(ctx).Debug().
		Str("entryPoint", job.Spec.Wasm.EntryPoint).
		Msg("Running WASM job")
	entryFunc := instance.ExportedFunction(job.Spec.Wasm.EntryPoint)
	exitCode := -1
	_, wasmErr := entryFunc.Call(ctx)
	var errExit *sys.ExitError
	if errors.As(wasmErr, &errExit) {
		exitCode = int(errExit.ExitCode())
		wasmErr = nil
	}

	return executor.WriteJobResults(jobResultsDir, stdout, stderr, exitCode, wasmErr)
}

func (e *Executor) GetOutputStream(context.Context, model.Job) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented for wasm executor")
}

// Compile-time check that Executor implements the Executor interface.
var _ executor.Executor = (*Executor)(nil)
