from __future__ import absolute_import
from __future__ import unicode_literals

import pytest

from .. import mock
from .. import unittest
from compose.plugin import Plugin
from compose.plugin import PluginJsonFileError


class PluginTest(unittest.TestCase):
    def setUp(self):
        self.invalid_plugin_info = {'var1': 'value1'}

    @mock.patch('compose.plugin.os')
    def test_load_plugin_info_from_file(self, mock_os):
        pass

    def test_check_required_plugin_file_settings(self):
        with pytest.raises(PluginJsonFileError):
            Plugin.check_required_plugin_file_settings(self.invalid_plugin_info, Plugin.required_fields)
