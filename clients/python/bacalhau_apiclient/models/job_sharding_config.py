# coding: utf-8

"""
    Bacalhau API

    This page is the reference of the Bacalhau REST API. Project docs are available at https://docs.bacalhau.org/. Find more information about Bacalhau at https://github.com/filecoin-project/bacalhau.  # noqa: E501

    OpenAPI spec version: 1.0.0
    Contact: team@bacalhau.org
    Generated by: https://github.com/swagger-api/swagger-codegen.git
"""


import pprint
import re  # noqa: F401

import six

from bacalhau_apiclient.configuration import Configuration


class JobShardingConfig(object):
    """NOTE: This class is auto generated by the swagger code generator program.

    Do not edit the class manually.
    """

    """
    Attributes:
      swagger_types (dict): The key is attribute name
                            and the value is attribute type.
      attribute_map (dict): The key is attribute name
                            and the value is json key in definition.
    """
    swagger_types = {
        'batch_size': 'int',
        'glob_pattern': 'str',
        'glob_pattern_base_path': 'str'
    }

    attribute_map = {
        'batch_size': 'BatchSize',
        'glob_pattern': 'GlobPattern',
        'glob_pattern_base_path': 'GlobPatternBasePath'
    }

    def __init__(self, batch_size=None, glob_pattern=None, glob_pattern_base_path=None, _configuration=None):  # noqa: E501
        """JobShardingConfig - a model defined in Swagger"""  # noqa: E501
        if _configuration is None:
            _configuration = Configuration()
        self._configuration = _configuration

        self._batch_size = None
        self._glob_pattern = None
        self._glob_pattern_base_path = None
        self.discriminator = None

        if batch_size is not None:
            self.batch_size = batch_size
        if glob_pattern is not None:
            self.glob_pattern = glob_pattern
        if glob_pattern_base_path is not None:
            self.glob_pattern_base_path = glob_pattern_base_path

    @property
    def batch_size(self):
        """Gets the batch_size of this JobShardingConfig.  # noqa: E501

        how many \"items\" are to be processed in each shard we first apply the glob pattern which will result in a flat list of items this number decides how to group that flat list into actual shards run by compute nodes  # noqa: E501

        :return: The batch_size of this JobShardingConfig.  # noqa: E501
        :rtype: int
        """
        return self._batch_size

    @batch_size.setter
    def batch_size(self, batch_size):
        """Sets the batch_size of this JobShardingConfig.

        how many \"items\" are to be processed in each shard we first apply the glob pattern which will result in a flat list of items this number decides how to group that flat list into actual shards run by compute nodes  # noqa: E501

        :param batch_size: The batch_size of this JobShardingConfig.  # noqa: E501
        :type: int
        """

        self._batch_size = batch_size

    @property
    def glob_pattern(self):
        """Gets the glob_pattern of this JobShardingConfig.  # noqa: E501

        divide the inputs up into the smallest possible unit for example /* would mean \"all top level files or folders\" this being an empty string means \"no sharding\"  # noqa: E501

        :return: The glob_pattern of this JobShardingConfig.  # noqa: E501
        :rtype: str
        """
        return self._glob_pattern

    @glob_pattern.setter
    def glob_pattern(self, glob_pattern):
        """Sets the glob_pattern of this JobShardingConfig.

        divide the inputs up into the smallest possible unit for example /* would mean \"all top level files or folders\" this being an empty string means \"no sharding\"  # noqa: E501

        :param glob_pattern: The glob_pattern of this JobShardingConfig.  # noqa: E501
        :type: str
        """

        self._glob_pattern = glob_pattern

    @property
    def glob_pattern_base_path(self):
        """Gets the glob_pattern_base_path of this JobShardingConfig.  # noqa: E501

        when using multiple input volumes what path do we treat as the common mount path to apply the glob pattern to  # noqa: E501

        :return: The glob_pattern_base_path of this JobShardingConfig.  # noqa: E501
        :rtype: str
        """
        return self._glob_pattern_base_path

    @glob_pattern_base_path.setter
    def glob_pattern_base_path(self, glob_pattern_base_path):
        """Sets the glob_pattern_base_path of this JobShardingConfig.

        when using multiple input volumes what path do we treat as the common mount path to apply the glob pattern to  # noqa: E501

        :param glob_pattern_base_path: The glob_pattern_base_path of this JobShardingConfig.  # noqa: E501
        :type: str
        """

        self._glob_pattern_base_path = glob_pattern_base_path

    def to_dict(self):
        """Returns the model properties as a dict"""
        result = {}

        for attr, _ in six.iteritems(self.swagger_types):
            value = getattr(self, attr)
            if isinstance(value, list):
                result[attr] = list(map(
                    lambda x: x.to_dict() if hasattr(x, "to_dict") else x,
                    value
                ))
            elif hasattr(value, "to_dict"):
                result[attr] = value.to_dict()
            elif isinstance(value, dict):
                result[attr] = dict(map(
                    lambda item: (item[0], item[1].to_dict())
                    if hasattr(item[1], "to_dict") else item,
                    value.items()
                ))
            else:
                result[attr] = value
        if issubclass(JobShardingConfig, dict):
            for key, value in self.items():
                result[key] = value

        return result

    def to_str(self):
        """Returns the string representation of the model"""
        return pprint.pformat(self.to_dict())

    def __repr__(self):
        """For `print` and `pprint`"""
        return self.to_str()

    def __eq__(self, other):
        """Returns true if both objects are equal"""
        if not isinstance(other, JobShardingConfig):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, JobShardingConfig):
            return True

        return self.to_dict() != other.to_dict()