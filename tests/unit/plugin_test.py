from __future__ import absolute_import
from __future__ import unicode_literals

import json
import re

import pytest

import tests
from .. import mock
from .. import unittest
from compose.cli.main import TopLevelCommand
from compose.plugin import compose_command
from compose.plugin import compose_patch
from compose.plugin import Plugin
from compose.plugin import PluginCommandError
from compose.plugin import PluginJsonFileError


class ClassToPatch(object):
    def __init__(self):
        self.name = "name"

    def get_name(self, prefix):
        return prefix + self.name


def test_compose_patch_class():
    @compose_patch(ClassToPatch, "get_name")
    def patched_get_name(self, original_fnc, prefix):
        original_return = original_fnc(self, prefix)
        return original_return + prefix[::-1]

    mock_instance = ClassToPatch()
    assert mock_instance.get_name("abc|") == "abc|name|cba"
    assert hasattr(mock_instance.get_name, '__standalone__') is False


def fnc_to_patch(string):
    return string

fnc_to_patch.__standalone__ = True


def test_compose_patch_function():
    @compose_patch(tests.unit.plugin_test, "fnc_to_patch")
    def patch_text_fnc(original_fnc, string):
        return original_fnc(string) * 2

    assert fnc_to_patch("|test|") == "|test||test|"
    assert fnc_to_patch.__standalone__ is True


def test_compose_command():
    @compose_command()
    def test_command(self):
        """
        Test command

        Usage: config [options]

        Options:
            -o, --option     Option
        """
        return True

    @compose_command(standalone=True)
    def stest_command(self):
        """
        Second test command

        Usage: config [options]

        Options:
            -o, --option     Option
        """
        return False

    if hasattr(TopLevelCommand, '__modified_doc__'):
        assert re.search(
            r'test_command\s+Test command',
            TopLevelCommand.__modified_doc__
        ) is not None
        assert re.search(
            r'stest_command\s+Second test command',
            TopLevelCommand.__modified_doc__
        ) is not None
    else:
        assert re.search(
            r'test_command\s+Test command',
            TopLevelCommand.__doc__
        ) is not None
        assert re.search(
            r'stest_command\s+Second test command',
            TopLevelCommand.__doc__
        ) is not None

    top_level_command = TopLevelCommand(None, None, None)

    assert TopLevelCommand.test_command.__standalone__ is False
    assert TopLevelCommand.stest_command.__standalone__ is True

    assert top_level_command.test_command() is True
    assert top_level_command.stest_command() is False

    with pytest.raises(PluginCommandError):
        test_command(None)

    with pytest.raises(PluginCommandError):
        stest_command(None)


class PluginTest(unittest.TestCase):
    valid_plugin_info = {'name': 'Plugin name', 'version': '1.0.0', 'description': 'Plugin description'}
    invalid_plugin_info = {'var1': 'value1'}

    def setUp(self):
        self.json_file = json.dumps(self.valid_plugin_info, False, True)

    def getPluginFileMock(self):
        return mock.mock_open(read_data=self.json_file)

    def _get_loaded_plugin(self):
        with mock.patch('compose.plugin.os.path.abspath') as mock_abspath, \
                mock.patch('compose.plugin.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin.open', self.getPluginFileMock(), create=True):
            mock_abspath.return_value = '/plugins/plugin_id'
            mock_isfile.return_value = True

            return Plugin(None, {})

    def test_init(self):
        with mock.patch('compose.plugin.os.path.abspath') as mock_abspath, \
                mock.patch('compose.plugin.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin.open', self.getPluginFileMock(), create=True):
            mock_abspath.return_value = '/plugins/plugin_id'
            mock_isfile.return_value = True

            self.assertIsInstance(Plugin(None, {}), Plugin)

    def test_check_required_plugin_file_settings_error(self):
        with self.assertRaises(PluginJsonFileError) as e:
            Plugin.check_required_plugin_file_settings(self.invalid_plugin_info, Plugin.required_fields)

        self.assertEqual(str(e.exception), "Missing json attribute 'name'")

    def test_check_required_plugin_file_settings_success(self):
        check = Plugin.check_required_plugin_file_settings(
            self.valid_plugin_info,
            Plugin.required_fields
        )

        self.assertEquals(check, True)

    def test_load_plugin_info_from_file_not_found(self):
        plugin = self._get_loaded_plugin()

        with self.assertRaises(PluginJsonFileError) as e:
            plugin.load_plugin_info_from_file('invalid.json')

        self.assertEqual(str(e.exception), "JSON plugin file not found")

    def test_load_plugin_info_from_file(self):
        plugin = self._get_loaded_plugin()

        with mock.patch('compose.plugin.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin.open', self.getPluginFileMock(), create=True):
            mock_isfile.return_value = True
            plugin.load_plugin_info_from_file('plugin.json')

            self.assertEquals(plugin.name, 'Plugin name')
            self.assertEquals(plugin.version, '1.0.0')
            self.assertEquals(plugin.description, 'Plugin description')

    def test_install(self):
        plugin = self._get_loaded_plugin()
        self.assertEquals(plugin.install(), True)

    def test_uninstall(self):
        plugin = self._get_loaded_plugin()
        self.assertEquals(plugin.uninstall(), True)

    def test_update(self):
        plugin = self._get_loaded_plugin()
        self.assertEquals(plugin.update(), None)

    def test_configure(self):
        plugin = self._get_loaded_plugin()
        self.assertEquals(plugin.configure(), None)
