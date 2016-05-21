# import importlib.util
from __future__ import absolute_import
from __future__ import unicode_literals

from .plugin import Plugin  # needed for building
import os
import shutil
import sys
import imp


class NoPluginError(Exception):
    pass


class PluginManager(object):
    def __init__(
        self,
        plugin_dir
    ):
        self.plugin_dir = plugin_dir
        self.plugin_list = {}

        for current_plugin_dir in os.listdir(self.plugin_dir):
            dir_with_path = os.path.join(self.plugin_dir, current_plugin_dir)

            if os.path.isdir(dir_with_path):
                init_file = os.path.join(dir_with_path, '__init__.py')

                if not os.path.isfile(init_file):
                    raise NoPluginError("Plugin at '{}' is has no ini file".format(dir_with_path))

                sys.path.append(dir_with_path)
                plugin_instance = imp.load_source(current_plugin_dir, init_file)

                '''
                spec = importlib.util.spec_from_file_location(current_plugin_dir, init_file)
                plugin_instance = importlib.util.module_from_spec(spec)
                spec.loader.exec_module(plugin_instance)
                '''

                if not hasattr(plugin_instance, 'plugin'):
                    raise NoPluginError("Plugin at '{}' is not a plugin".format(current_plugin_dir))

                if not isinstance(plugin_instance.plugin, Plugin):
                    raise NoPluginError("Plugin at '{}' is not a plugin".format(current_plugin_dir))

                self.plugin_list[current_plugin_dir] = plugin_instance.plugin

    def get_plugins(self):
        return self.plugin_list

    def __plugin_exists(self, name):
        if name not in self.plugin_list:
            raise NoPluginError("Plugin {} doesn't exists".format(name))

    def is_plugin_installed(self, name):
        self.__plugin_exists(name)
        return self.plugin_list[name].is_installed()


    def install_plugin(self, name):
        self.__plugin_exists(name)
        return self.plugin_list[name].install()

    def uninstall_plugin(self, name):
        self.__plugin_exists(name)

        # After the uninstall method was successful remove the plugin
        if self.plugin_list[name].uninstall():
            return shutil.rmtree(self.plugin_list[name].path)

        return False

    def configure_plugin(self, name):
        self.__plugin_exists(name)
        return self.plugin_list[name].configure()
