package devstack

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/filecoin-project/bacalhau/pkg/docker"
	"github.com/filecoin-project/bacalhau/pkg/storage/util"
	"github.com/filecoin-project/bacalhau/pkg/system"
	"github.com/filecoin-project/bacalhau/pkg/util/closer"
	"github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
)

const defaultImage = "ghcr.io/bacalhau-project/lotus-filecoin-image:v0.0.1"

type LotusNode struct {
	client    *dockerclient.Client
	image     string
	container string

	// UploadDir is the directory where files to be uploaded to Lotus should be stored
	UploadDir string
	// PathDir is the directory will be used as `$LOTUS_PATH`, containing various bits of config
	PathDir string
}

func newLotusNode(ctx context.Context) (*LotusNode, error) {
	ctx, span := system.GetTracer().Start(ctx, "pkg/devstack.newlotusnode")
	defer span.End()

	image := defaultImage
	if e, ok := os.LookupEnv("LOTUS_TEST_IMAGE"); ok {
		image = e
	}

	dockerClient, err := docker.NewDockerClient()
	if err != nil {
		return nil, err
	}

	if err := docker.PullImage(ctx, dockerClient, image); err != nil {
		closer.CloseWithLogOnError("docker", dockerClient)
		return nil, err
	}

	return &LotusNode{
		client: dockerClient,
		image:  image,
	}, nil
}

// start performs the work of actually starting the Lotus container. This is separated from the constructor so the user
// can cancel and still have the container, which may not be healthy yet, cleaned up via Close.
func (l *LotusNode) start(ctx context.Context) error {
	ctx, span := system.GetTracer().Start(ctx, "pkg/devstack.start")
	defer span.End()

	uploadDir, err := ioutil.TempDir("", "bacalhau-lotus-upload-dir")
	if err != nil {
		return err
	}
	l.UploadDir = uploadDir

	pathDir, err := ioutil.TempDir("", "bacalhau-lotus-path-dir")
	if err != nil {
		return err
	}
	l.PathDir = pathDir

	c, err := l.client.ContainerCreate(ctx, &container.Config{
		Image: l.image,
	}, &container.HostConfig{
		PortBindings: map[nat.Port][]nat.PortBinding{
			"1234/tcp": {{}},
		},
		Mounts: []mount.Mount{
			// Mount the temp directory at the same place within the container to aviod confusion between paths outside the
			// container, that the user sees, and paths within the container, that the ClientImport command uses.
			{
				Type:     mount.TypeBind,
				ReadOnly: true,
				Source:   l.UploadDir,
				Target:   l.UploadDir,
			},
		},
	}, nil, nil, "")
	if err != nil {
		return err
	}

	l.container = c.ID

	log.Debug().
		Str("image", l.image).
		Str("UploadDir", l.UploadDir).
		Str("PathDir", l.PathDir).
		Str("containerId", l.container).
		Msg("Starting Lotus container")

	if err := l.client.ContainerStart(ctx, l.container, dockertypes.ContainerStartOptions{}); err != nil {
		return err
	}

	if err := l.waitForLotusToBeHealthy(ctx); err != nil {
		if err := l.Close(); err != nil { //nolint:govet
			log.Err(err).Msgf(`Problem occurred when giving up waiting for Lotus to become healthy`)
		}
		return err
	}

	return nil
}

func (l *LotusNode) waitForLotusToBeHealthy(ctx context.Context) error {
	ctx, span := system.GetTracer().Start(ctx, "pkg/devstack.waitforlotustobehealthy")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute) //nolint:gomnd
	defer cancel()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		state, err := l.client.ContainerInspect(ctx, l.container)
		if err != nil {
			return err
		}

		if state.State.Health.Status == dockertypes.Healthy {
			if err := l.writeConfigToml(state.NetworkSettings.Ports["1234/tcp"][0].HostPort); err != nil {
				return err
			}
			break
		}

		e := log.Debug()
		if len(state.State.Health.Log) != 0 {
			e = e.Str("last-health-check", strings.TrimSpace(state.State.Health.Log[len(state.State.Health.Log)-1].Output))
		}
		e.Msg("Lotus not healthy yet")
		time.Sleep(5 * time.Second) //nolint:gomnd
	}

	if err := l.copyOutTokenFile(ctx); err != nil {
		return err
	}

	return nil
}

func (l *LotusNode) copyOutTokenFile(ctx context.Context) error {
	content, _, err := l.client.CopyFromContainer(ctx, l.container, "/home/lotus_user/.lotus-local-net/token")
	if err != nil {
		return err
	}

	defer closer.CloseWithLogOnError("content", content)

	tarContent := tar.NewReader(content)
	if _, err := tarContent.Next(); err != nil { //nolint:govet
		return err
	}

	tokenFile, err := os.OpenFile(filepath.Join(l.PathDir, "token"), os.O_CREATE|os.O_WRONLY, util.OS_USER_RW)
	if err != nil {
		return err
	}

	defer closer.CloseWithLogOnError("token-file", tokenFile)

	if _, err := io.Copy(tokenFile, tarContent); err != nil { //nolint:gosec // This can't DoS as it's writing to a file
		return err
	}

	return nil
}

func (l *LotusNode) writeConfigToml(port string) error {
	config := fmt.Sprintf(`#https://lotus.filecoin.io/lotus/configure/defaults/
[API]
ListenAddress = "/ip4/0.0.0.0/tcp/%s/http"
`, port)

	if err := os.WriteFile(filepath.Join(l.PathDir, "config.toml"), []byte(config), util.OS_USER_RW); err != nil {
		return err
	}

	return nil
}

func (l *LotusNode) Close() error {
	var errs error

	defer closer.CloseWithLogOnError("Docker client", l.client)
	if l.container != "" {
		if err := docker.RemoveContainer(context.Background(), l.client, l.container); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	if l.UploadDir != "" {
		if err := os.RemoveAll(l.UploadDir); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	if l.PathDir != "" {
		if err := os.RemoveAll(l.PathDir); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	if errs != nil {
		return errs
	}

	return nil
}