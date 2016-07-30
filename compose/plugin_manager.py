from __future__ import absolute_import
from __future__ import unicode_literals

import imp
import os
import shutil
import tarfile

from compose.cli.command import get_config_path_from_options
from compose.config import config
from compose.config.environment import Environment
from compose.config.errors import ComposeFileNotFound

try:
    import urllib.request as request
except ImportError:
    import urllib2 as request

import zipfile

from .plugin import Plugin
from .plugin import PluginError


class PluginDoesNotExistError(PluginError):
    pass


class InvalidPluginError(PluginError):
    pass


class InvalidPluginFileTypeError(PluginError):
    pass


class NoneLoadedConfigError(PluginError):
    pass


class PluginManager(object):
    def __init__(self, plugin_dir):
        self.plugin_dir = plugin_dir
        self.__plugin_download_dir = os.path.join(self.plugin_dir, '.downloads')
        self.config = None
        self.plugin_classes = None
        self.plugin_list = None

        self.plugin_classes = self.__get_plugin_classes()

    def load_config(self, project_dir, options):
        try:
            environment = Environment.from_env_file(project_dir)
            config_path = get_config_path_from_options(project_dir, options, environment)
            config_details = config.find(project_dir, config_path, environment)
            self.config = config.load(config_details)
        except ComposeFileNotFound:
            self.config = False

        self.__load_plugins()

    def __get_plugin_paths(self):
        paths = {}

        if os.path.isdir(self.plugin_dir):
            for current_plugin_dir in os.listdir(self.plugin_dir):
                plugin_path = os.path.join(self.plugin_dir, current_plugin_dir)

                if os.path.isdir(plugin_path):
                    paths[current_plugin_dir] = plugin_path

        return paths

    @staticmethod
    def __source_plugin(path):
        current_plugin_dir = os.path.basename(path)
        init_file = os.path.join(path, '__init__.py')

        if not os.path.isfile(init_file):
            raise InvalidPluginError(
                "Missing __init__.py file."
            )

        plugin_package = imp.load_source(current_plugin_dir, init_file)

        if not hasattr(plugin_package, 'plugin'):
            raise InvalidPluginError(
                "Plugin '{}' is not a plugin. Missing plugin attribute.".format(current_plugin_dir)
            )

        if not issubclass(plugin_package.plugin, Plugin):
            raise InvalidPluginError(
                "Wrong plugin instance.".format(current_plugin_dir)
            )

        return plugin_package.plugin

    def __get_plugin_classes(self):
        if self.plugin_classes is None:
            self.plugin_classes = {}

            for (plugin_id, plugin_path) in self.__get_plugin_paths().items():
                try:
                    plugin = self.__source_plugin(plugin_path)
                    self.plugin_classes[plugin_id] = plugin
                except InvalidPluginError:
                    print("Invalid plugin '{}' installed".format(plugin_id))
                except TypeError as e:
                    print("Invalid plugin error: {}".format(str(e)))

        return self.plugin_classes

    def __load_plugins(self):
        if self.plugin_list is None:
            if self.config is None:
                raise NoneLoadedConfigError("The configuration wan't loaded for the plugin manager. "
                                            "Plugins can only instantiated after that.")

            plugins_config = self.config.plugins if self.config is not False else {}
            self.plugin_list = {}

            for (plugin_id, plugin_class) in self.__get_plugin_classes().items():
                plugin_config = plugins_config[plugin_id] if plugin_id in plugins_config else {}
                plugin_instance = plugin_class(self, plugin_config)
                self.plugin_list[plugin_id] = plugin_instance

        return self.plugin_list

    def get_plugins(self):
        return self.__load_plugins()

    def __plugin_exists(self, plugin_id):
        if id not in self.get_plugins():
            raise PluginDoesNotExistError("Plugin '{}' doesn't exists".format(plugin_id))

    def is_plugin_installed(self, plugin_id):
        try:
            self.__plugin_exists(plugin_id)
            return True
        except PluginDoesNotExistError:
            return False

    def __get_plugin_file(self, plugin):
        try:
            file = os.path.join(self.__plugin_download_dir, os.path.basename(plugin))
            request.urlretrieve(plugin, file)
        except ValueError:  # invalid URL
            file = os.path.realpath(plugin)

            if not os.path.isfile(file):
                return False

        return file

    def __check_plugin_archive(self, file):
        if zipfile.is_zipfile(file):
            archive = zipfile.ZipFile(file)
        elif tarfile.is_tarfile(file):
            archive = tarfile.TarFile(file)
        else:
            raise InvalidPluginFileTypeError('Invalid file type.')

        plugin_folder = None

        # TODO improve check
        for file in archive.namelist():
            if file.endswith('plugin.json'):
                plugin_folder = os.path.dirname(file)
                break

        if plugin_folder is None:
            raise InvalidPluginFileTypeError('Missing plugin.json file.')

        archive.extractall(self.plugin_dir)
        return os.path.join(self.plugin_dir, plugin_folder)

    def install_plugin(self, plugin):
        file = self.__get_plugin_file(plugin)

        if not os.path.isdir(self.plugin_dir):
            os.makedirs(self.plugin_dir)

        plugin_path = self.__check_plugin_archive(file)

        try:
            plugin_class = self.__source_plugin(plugin_path)
            plugin_instance = plugin_class(self, {})
            plugin_instance.install()
        except InvalidPluginError as e:
            shutil.rmtree(plugin_path)
            raise e

        if os.path.isdir(self.__plugin_download_dir):
            shutil.rmtree(self.__plugin_download_dir)

    def uninstall_plugin(self, name):
        self.__plugin_exists(name)

        # After the uninstall method was successful remove the plugin
        if self.plugin_list[name].uninstall():
            return shutil.rmtree(self.plugin_list[name].path)

        return False

    def update_plugin(self, plugin_id):
        self.__plugin_exists(plugin_id)
        return self.plugin_list[plugin_id].update()

    def configure_plugin(self, plugin_id):
        self.__plugin_exists(plugin_id)
        return self.plugin_list[plugin_id].configure()
