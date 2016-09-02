from __future__ import absolute_import
from __future__ import unicode_literals

from .. import mock
from .. import unittest
from compose.plugin import Plugin
from compose.plugin_manager import InvalidPluginError
from compose.plugin_manager import InvalidPluginFileTypeError
from compose.plugin_manager import NoneLoadedConfigError
from compose.plugin_manager import PluginDoesNotExistError
from compose.plugin_manager import PluginManager
from compose.plugin_manager import PluginRequirementsError


class PluginManagerTest(unittest.TestCase):
    def setUp(self):
        pass

    def _get_helper_class(self, attributes=None):
        class Foo(object):
            def __init__(self):
                if attributes is not None:
                    for (attribute_name, attribute_value) in attributes.items():
                        setattr(self, attribute_name, attribute_value)

        return Foo()

    def _get_archive_mock(self, files):
        class MockArchive:
            def __init__(self, archive_files):
                self.files = archive_files

            def __iter__(self):
                return iter(self.files)

            def write(self, fname):
                self.files.append(fname)

            def namelist(self):
                return self.files

            def extractall(self, destination):
                pass

        return MockArchive(files)

    @staticmethod
    def _get_plugin_manager_with_plugin():
        plugin_manager = PluginManager('plugin_dir')

        plugin_copy = Plugin
        plugin_copy.__init__ = lambda a, b, c: None
        plugin_copy.path = 'path'
        plugin_copy.version = '0.0.1'
        plugin_manager.plugin_list = {
            'plugin': plugin_copy(plugin_manager, {})
        }

        return plugin_manager

    def test_load_config(self):
        plugin_manager = PluginManager('')
        plugin_manager.load_config('', {})
        self.assertEquals(plugin_manager.config, False)

    def test_get_plugin_paths_invalid_dir(self):
        with mock.patch('compose.plugin_manager.os.path.isdir') as mock_isdir:
            mock_isdir.return_value = False
            plugin_manager = PluginManager('plugin_dir')
            self.assertEquals(plugin_manager._get_plugin_paths(), {})

    def test_get_plugin_paths_valid_dir(self):
        with mock.patch('compose.plugin_manager.os.path.isdir') as mock_isdir,\
                mock.patch('compose.plugin_manager.os.listdir') as mock_listdir:
            plugin_manager = PluginManager('')
            plugin_manager.plugin_dir = 'plugin_dir'
            mock_isdir.side_effect = [True, True, False, True]
            mock_listdir.return_value = ['plugin_1', 'plugin_2', 'plugin_3']

            self.assertEquals(
                plugin_manager._get_plugin_paths(),
                {
                    'plugin_1': 'plugin_dir/plugin_1',
                    'plugin_3': 'plugin_dir/plugin_3'
                }
            )

    def test_source_plugin_missing_init_file(self):
        plugin_manager = PluginManager('')

        with mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile:
            mock_isfile.return_value = False

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager._source_plugin('plugin_path')

            self.assertEqual(str(e.exception), "Missing __init__.py file.")

    def test_source_plugin_missing_plugin_attribute(self):
        plugin_manager = PluginManager('')

        with mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile,\
                mock.patch('imp.load_source') as mock_load_source, \
                mock.patch('os.walk') as mock_walk:
            mock_walk.return_value = []
            mock_load_source.return_value = self._get_helper_class()
            mock_isfile.return_value = True

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager._source_plugin('plugin')

            self.assertEqual(
                str(e.exception),
                "Plugin 'plugin' is not a plugin. Missing plugin attribute."
            )

    def test_source_plugin_invalid_plugin_attribute(self):
        plugin_manager = PluginManager('')

        with mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile, \
                mock.patch('imp.load_source') as mock_load_source, \
                mock.patch('os.walk') as mock_walk:
            mock_walk.return_value = []
            mock_load_source.return_value = self._get_helper_class({
                'plugin': self._get_helper_class().__class__
            })
            mock_isfile.return_value = True

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager._source_plugin('plugin')

        self.assertEqual(str(e.exception), "Wrong plugin instance.")

    def test_source_plugin_valid_plugin(self):
        plugin_manager = PluginManager('')

        with mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile, \
                mock.patch('imp.load_source') as mock_load_source, \
                mock.patch('os.walk') as mock_walk:
            mock_walk.return_value = []
            mock_load_source.return_value = self._get_helper_class({
                'plugin': Plugin
            })
            mock_isfile.return_value = True

            self.assertEqual(plugin_manager._source_plugin('plugin'), Plugin)

    def test_get_plugin_classes_invalid_dir(self):
        plugin_manager = PluginManager('')
        self.assertEquals(plugin_manager._get_plugin_classes(), {})

    def test_get_plugin_classes_valid_plugins(self):
        with mock.patch('compose.plugin_manager.os.path.isdir') as mock_isdir, \
                mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin_manager.os.listdir') as mock_listdir, \
                mock.patch('imp.load_source') as mock_load_source, \
                mock.patch('os.walk') as mock_walk:
            mock_walk.return_value = []
            plugin_manager = PluginManager('')
            plugin_manager.plugin_classes = None
            mock_load_source.return_value = self._get_helper_class({
                'plugin': Plugin
            })
            mock_isdir.return_value = True
            mock_isfile.return_value = True
            mock_listdir.return_value = ['plugin_1', 'plugin_2']

            self.assertEquals(plugin_manager._get_plugin_classes(), {
                'plugin_1': Plugin,
                'plugin_2': Plugin
            })

    def test_check_required_plugins(self):
        with mock.patch.object(Plugin, '__init__') as mock_plugin:
            mock_plugin.return_value = None
            plugin_manager = PluginManager('')

            plugins = {}
            plugins_config = {
                'plugin_1': {},
                'plugin_2': {}
            }

            with self.assertRaises(PluginRequirementsError) as e:
                plugin_manager._check_required_plugins(plugins_config, plugins)

            self.assertEqual(
                str(e.exception),
                "Missing required plugins: 'plugin_1', 'plugin_2'"
            )

            plugin_1 = Plugin(plugin_manager, {})
            plugin_1.version = '0.0.1'

            plugin_2 = Plugin(plugin_manager, {})
            plugin_2.version = '2.0.1'

            plugins = {
                'plugin_1': plugin_1,
                'plugin_2': plugin_2
            }
            plugins_config = {
                'plugin_1': {
                    'version': '2.0.0'
                },
                'plugin_2': {}
            }

            with self.assertRaises(PluginRequirementsError) as e:
                plugin_manager._check_required_plugins(plugins_config, plugins)

            self.assertEqual(
                str(e.exception),
                "Plugin 'plugin_1' must at least version '2.0.0'"
            )

            plugins_config = {
                'plugin_1': {
                    'version': '0.0.1'
                },
                'plugin_2': {
                    'version': '1.0.0'
                }
            }

            result = plugin_manager._check_required_plugins(plugins_config, plugins)
            self.assertEquals(result, None)

    def test_load_plugins_none_loaded_config(self):
        plugin_manager = PluginManager('')

        with self.assertRaises(NoneLoadedConfigError) as e:
            plugin_manager._load_plugins()

        self.assertEquals(
            str(e.exception),
            "The configuration wasn't loaded for the plugin manager. "
            "Plugins can only instantiated after that."
        )

    def test_load_plugins(self):
        with mock.patch('compose.plugin_manager.os.path.isdir') as mock_isdir, \
                mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin_manager.os.listdir') as mock_listdir, \
                mock.patch('imp.load_source') as mock_load_source, \
                mock.patch.object(Plugin, '__init__') as mock_plugin, \
                mock.patch('os.walk') as mock_walk:
            mock_walk.return_value = []
            mock_plugin.return_value = None

            plugin_manager = PluginManager('')
            plugin_manager.plugin_classes = None

            mock_load_source.return_value = self._get_helper_class({
                'plugin': Plugin
            })
            mock_isdir.return_value = True
            mock_isfile.return_value = True
            mock_listdir.return_value = ['plugin_1', 'plugin_2']
            plugin_manager.load_config('', {})

            loaded_plugins = plugin_manager._load_plugins()
            self.assertEquals('plugin_1' in loaded_plugins.keys(), True)
            self.assertEquals('plugin_2' in loaded_plugins.keys(), True)
            self.assertEquals(isinstance(loaded_plugins['plugin_1'], Plugin), True)
            self.assertEquals(isinstance(loaded_plugins['plugin_2'], Plugin), True)

    """
    def test_get_plugins(self):
        assert False
    """

    def get_loaded_plugin_manager(self):
        with mock.patch('compose.plugin_manager.os.path.isdir') as mock_isdir, \
                mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin_manager.os.listdir') as mock_listdir, \
                mock.patch('imp.load_source') as mock_load_source, \
                mock.patch.object(Plugin, '__init__') as mock_plugin, \
                mock.patch('os.walk') as mock_walk:
            mock_walk.return_value = []
            mock_plugin.return_value = None
            mock_load_source.return_value = self._get_helper_class({
                'plugin': Plugin
            })
            mock_isdir.return_value = True
            mock_isfile.return_value = True
            mock_listdir.return_value = ['plugin_1', 'plugin_2']

            plugin_manager = PluginManager('plugin_folder')
            plugin_manager.load_config('', {})
            return plugin_manager

    def test_plugin_exists(self):
        plugin_manager = self.get_loaded_plugin_manager()

        with self.assertRaises(PluginDoesNotExistError) as e:
            plugin_manager._plugin_exists('no_plugin')

        self.assertEqual(str(e.exception), "Plugin 'no_plugin' doesn't exists")
        plugin_manager._plugin_exists('plugin_1')

    def test_is_plugin_installed(self):
        plugin_manager = self.get_loaded_plugin_manager()

        self.assertEquals(plugin_manager.is_plugin_installed('plugin_1'), True)
        self.assertEquals(plugin_manager.is_plugin_installed('no_plugin'), False)

    def test_get_plugin_file(self):
        with mock.patch('compose.plugin_manager.request.urlretrieve') as mock_urlretrieve, \
                mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin_manager.os.path.realpath') as mock_realpath, \
                mock.patch('os.mkdir') as mock_mkdir:
            plugin_manager = PluginManager('plugin_dir')

            self.assertEquals(
                plugin_manager._get_plugin_file('plugin_name'),
                'plugin_dir/.downloads/plugin_name'
            )

            mock_urlretrieve.side_effect = ValueError()
            mock_isfile.return_value = False
            mock_mkdir.return_value = True

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager._get_plugin_file('no_plugin')

            self.assertEqual(str(e.exception), "Invalid plugin url or file given.")

            mock_isfile.return_value = True
            mock_realpath.return_value = '/real/path/to/plugin/plugin_name'

            self.assertEquals(
                plugin_manager._get_plugin_file('plugin_name'),
                '/real/path/to/plugin/plugin_name'
            )

    def test_check_plugin_archive(self):
        with mock.patch('compose.plugin_manager.zipfile.is_zipfile') as mock_is_zipfile, \
                mock.patch('compose.plugin_manager.zipfile.ZipFile') as mock_zipfile, \
                mock.patch('compose.plugin_manager.tarfile.is_tarfile') as mock_is_tarfile, \
                mock.patch('compose.plugin_manager.tarfile.TarFile') as mock_tarfile:
            plugin_manager = PluginManager('plugin_dir')
            mock_is_zipfile.return_value = False
            mock_is_tarfile.return_value = False
            with self.assertRaises(InvalidPluginFileTypeError) as e:
                plugin_manager._check_plugin_archive('no_plugin')

            self.assertEqual(str(e.exception), "Invalid file type.")

            mock_is_zipfile.return_value = True
            mock_zipfile.return_value = self._get_archive_mock([])

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager._check_plugin_archive('no_plugin')

            self.assertEqual(str(e.exception), "Wrong plugin structure.")

            mock_zipfile.return_value = self._get_archive_mock([
                'root_dir/',
                'second_root_dir/',
                'root_dir/plugin.json'
            ])

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager._check_plugin_archive('plugin.zip')

            self.assertEqual(str(e.exception), "Wrong plugin structure.")

            mock_zipfile.return_value = self._get_archive_mock([
                'root_dir/',
                'root_dir/plugin.js'
            ])

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager._check_plugin_archive('plugin.zip')

            self.assertEqual(str(e.exception), "Missing plugin.json file.")

            mock_zipfile.return_value = self._get_archive_mock([
                'root_dir/',
                'root_dir/plugin.json'
            ])

            result = plugin_manager._check_plugin_archive('plugin.zip')
            self.assertEqual(result, "plugin_dir/root_dir")

            mock_is_zipfile.return_value = False
            mock_is_tarfile.return_value = True
            mock_tarfile.return_value = self._get_archive_mock([
                'root_dir/',
                'root_dir/plugin.json'
            ])

            result = plugin_manager._check_plugin_archive('plugin.zip')
            self.assertEqual(result, "plugin_dir/root_dir")

    def test_install_plugin(self):
        with mock.patch('compose.plugin_manager.request.urlretrieve') as mock_urlretrieve, \
                mock.patch('compose.plugin_manager.os.path.isfile') as mock_isfile, \
                mock.patch('compose.plugin_manager.os.path.realpath') as mock_realpath, \
                mock.patch('compose.plugin_manager.os.path.isdir') as mock_isdir, \
                mock.patch('compose.plugin_manager.os.makedirs') as mock_makedirs, \
                mock.patch('compose.plugin_manager.zipfile.is_zipfile') as mock_is_zipfile, \
                mock.patch('compose.plugin_manager.tarfile.is_tarfile') as mock_is_tarfile, \
                mock.patch('compose.plugin_manager.zipfile.ZipFile') as mock_zipfile, \
                mock.patch('imp.load_source') as mock_load_source, \
                mock.patch('shutil.rmtree') as mock_rmtree, \
                mock.patch('os.mkdir') as mock_mkdir, \
                mock.patch('os.walk') as mock_walk:
            mock_walk.return_value = []
            mock_urlretrieve.side_effect = ValueError()
            mock_isfile.return_value = True
            mock_mkdir.return_value = True
            mock_realpath.return_value = '/real/path/to/plugin/plugin_name'

            mock_isdir.return_value = False

            mock_is_tarfile.return_value = False
            mock_is_zipfile.return_value = True
            mock_zipfile.return_value = self._get_archive_mock([
                'root_dir/',
                'root_dir/plugin.json'
            ])
            mock_load_source.return_value = None
            self._get_helper_class({
                'plugin': Plugin
            })

            plugin_manager = PluginManager('plugin_dir')

            with self.assertRaises(InvalidPluginError) as e:
                plugin_manager.install_plugin('plugin')
                self.assertTrue(mock_makedirs.called)
                self.assertTrue(mock_rmtree.called)

            self.assertEqual(
                str(e.exception),
                "Plugin 'root_dir' is not a plugin. Missing plugin attribute."
            )

            plugin_copy = Plugin
            plugin_copy.__init__ = lambda a, b, c: None
            plugin_copy.install = lambda a: False

            mock_load_source.return_value = self._get_helper_class({
                'plugin': plugin_copy
            })

            self.assertEquals(plugin_manager.install_plugin('plugin'), False)

            plugin_copy.install = lambda a: True

            mock_isdir.return_value = True
            mock_load_source.return_value = self._get_helper_class({
                'plugin': plugin_copy
            })

            self.assertEquals(plugin_manager.install_plugin('plugin'), True)

    def test_uninstall_plugin(self):
        with mock.patch('shutil.rmtree') as mock_rmtree:
            plugin_manager = self._get_plugin_manager_with_plugin()

            with self.assertRaises(PluginDoesNotExistError) as e:
                plugin_manager.uninstall_plugin('none_plugin')

            self.assertEqual(
                str(e.exception),
                "Plugin 'none_plugin' doesn't exists"
            )

            self.assertTrue(plugin_manager.uninstall_plugin('plugin'))
            self.assertTrue(mock_rmtree.called)

    def test_update_plugin(self):
        plugin_manager = self._get_plugin_manager_with_plugin()

        with self.assertRaises(PluginDoesNotExistError) as e:
            plugin_manager.update_plugin('none_plugin')

        self.assertEqual(
            str(e.exception),
            "Plugin 'none_plugin' doesn't exists"
        )

        self.assertEquals(plugin_manager.update_plugin('plugin'), None)

    def test_configure_plugin(self):
        plugin_manager = self._get_plugin_manager_with_plugin()

        with self.assertRaises(PluginDoesNotExistError) as e:
            plugin_manager.configure_plugin('none_plugin')

        self.assertEqual(
            str(e.exception),
            "Plugin 'none_plugin' doesn't exists"
        )

        self.assertEquals(plugin_manager.configure_plugin('plugin'), None)
