from __future__ import absolute_import
from __future__ import unicode_literals

import imp
import os
import shutil
import sys
import tarfile
import urllib.request as request
import zipfile

from .plugin import Plugin
from .plugin import PluginError


class PluginDoesNotExistError(PluginError):
    pass


class InvalidPluginError(PluginError):
    pass


class InvalidPluginFileTypeError(PluginError):
    pass


class PluginManager(object):
    def __init__(self, plugin_dir):
        self.plugin_dir = plugin_dir
        self.__plugin_download_dir = os.path.join(self.plugin_dir, '.downloads')
        self.plugin_list = {}

        if os.path.isdir(plugin_dir):
            for current_plugin_dir in os.listdir(self.plugin_dir):
                plugin_path = os.path.join(self.plugin_dir, current_plugin_dir)

                if os.path.isdir(plugin_path):
                    try:
                        self.__load_plugin(plugin_path)
                    except InvalidPluginError:
                        print("Invalid plugin '{}' installed".format(current_plugin_dir))

    def __load_plugin(self, path):
        current_plugin_dir = os.path.basename(path)
        init_file = os.path.join(path, '__init__.py')

        if not os.path.isfile(init_file):
            raise InvalidPluginError(
                "Missing __init__.py file."
            )

        sys.path.append(path)  # doesn't work with pyinstaller :(
        plugin_instance = imp.load_source(current_plugin_dir, init_file)

        if not hasattr(plugin_instance, 'plugin'):
            raise InvalidPluginError(
                "Plugin '{}' is not a plugin. Missing plugin attribute.".format(current_plugin_dir)
            )

        if not isinstance(plugin_instance.plugin, Plugin):
            raise InvalidPluginError(
                "Wrong plugin instance.".format(current_plugin_dir)
            )

        self.plugin_list[current_plugin_dir] = plugin_instance.plugin
        return self.plugin_list[current_plugin_dir]

    def __plugin_exists(self, name):
        if name not in self.plugin_list:
            raise PluginDoesNotExistError("Plugin '{}' doesn't exists".format(name))

    def get_plugins(self):
        return self.plugin_list

    def is_plugin_installed(self, name):
        try:
            self.__plugin_exists(name)
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

        # TODO better check
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
            self.__load_plugin(plugin_path).install()
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

    def update_plugin(self, name):
        self.__plugin_exists(name)
        return self.plugin_list[name].update()

    def configure_plugin(self, name):
        self.__plugin_exists(name)
        return self.plugin_list[name].configure()
