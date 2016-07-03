from __future__ import absolute_import
from __future__ import unicode_literals

import inspect
import json
import os


class PluginError(Exception):
    def __init__(self, *message, **errors):
        # Call the base class constructor with the parameters it needs
        super(PluginError, self).__init__(message, errors)

        self.message = message

    def __get_message(self):
        return self.message


class PluginJsonFileError(PluginError):
    pass


class PluginNotImplementError(PluginError):
    pass


class Plugin(object):
    required_fields = ['name', 'version']

    def __init__(self, config=None):
        self.config = config
        file = os.path.abspath(inspect.getfile(self.__class__))
        self.path = os.path.dirname(file)
        self.name = os.path.basename(self.path)
        self.description = ''
        self.version = None

        plugin_file = os.path.join(self.path, 'plugin.json')
        self.load_plugin_info_from_file(plugin_file)

    @staticmethod
    def check_required_plugin_file_settings(plugin_info, required_keys):
        for required_key in required_keys:
            if required_key not in plugin_info:
                raise PluginJsonFileError("Missing json attribute '{}'".format(required_key))

        return True

    def load_plugin_info_from_file(self, file):
        if os.path.isfile(file):
            with open(file) as f:
                plugin_info = json.load(f)

                self.check_required_plugin_file_settings(plugin_info, self.required_fields)
                self.name = plugin_info['name']
                self.description = plugin_info['description'] if 'description' in plugin_info else ''
                self.version = plugin_info['version']
        else:
            raise PluginJsonFileError('JSON plugin file not found')

    def install(self):
        pass

    def uninstall(self):
        pass

    def update(self):
        pass

    def configure(self):
        print("'{}' needs no configuration".format(self.name))

    def execute(self):
        raise PluginNotImplementError("Method execute for '{}' must be implemented".format(self.name))
