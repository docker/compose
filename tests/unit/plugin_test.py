from __future__ import absolute_import
from __future__ import unicode_literals

import json

import pytest

from .. import mock
from .. import unittest
from compose.plugin import Plugin
from compose.plugin import PluginJsonFileError


class PluginTest(unittest.TestCase):
    valid_plugin_info = {'name': 'Plugin name', 'version': '1.0.0', 'description': 'Plugin description'}
    invalid_plugin_info = {'var1': 'value1'}

    def setUp(self):
        pass

    @mock.patch('compose.plugin.os.path.isfile')
    def test_load_plugin_info_from_file(self, mock_isfile):
        mock_isfile.return_value = True
        json_file = json.dumps(self.valid_plugin_info, False, True)
        m = mock.mock_open(read_data=json_file)

        with mock.patch('compose.plugin.open', m, create=True):
            Plugin.__init__ = mock.Mock(return_value=None)
            plugin = Plugin(None)  # TODO plugin_manager mock
            plugin.load_plugin_info_from_file('plugin.json')

            assert plugin.name == 'Plugin name'
            assert plugin.version == '1.0.0'
            assert plugin.description == 'Plugin description'

    def test_check_required_plugin_file_settings_error(self):
        with pytest.raises(PluginJsonFileError):
            Plugin.check_required_plugin_file_settings(self.invalid_plugin_info, Plugin.required_fields)

    def test_check_required_plugin_file_settings_success(self):
        check = Plugin.check_required_plugin_file_settings(
            self.valid_plugin_info,
            Plugin.required_fields
        )

        assert check is True
