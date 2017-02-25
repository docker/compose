from __future__ import absolute_import
from __future__ import unicode_literals

import collections
import inspect
import json
import os
import re
from functools import partial

import compose


class PartialMethod(partial):
    def __get__(self, instance, owner):
        if instance is None:
            return self

        return partial(
            self.func,
            instance,
            *(self.args or ()),
            **(self.keywords or {})
        )


def compose_patch(scope, name):
    def wrapper(fnc):
        original = getattr(scope, name)

        if fnc.__doc__ is None:
            fnc.__doc__ = original.__doc__

        patched = PartialMethod(fnc, original)
        patched.__doc__ = fnc.__doc__

        if hasattr(original, '__standalone__'):
            patched.__standalone__ = original.__standalone__

        setattr(scope, name, patched)
        return fnc
    return wrapper


def compose_command(standalone=False):
    def update_command_doc(original_doc, fnc_name, fnc_doc):
        pre_doc = ''
        command_regex = r'(\s*)([^ ]+)(\s*)(.*)'
        doc_commands = None

        for compose_doc_line in original_doc.splitlines():
            if doc_commands is not None and re.match(command_regex, compose_doc_line):
                command = re.search(command_regex, compose_doc_line)
                doc_commands[command.group(2)] = compose_doc_line

                if fnc_name not in doc_commands:
                    space_to_text = len(command.group(2) + command.group(3))
                    new_command = command.group(1) + fnc_name
                    new_command += (' ' * (space_to_text - len(fnc_name)))
                    new_command += fnc_doc.strip(' \t\n\r').splitlines()[0]
                    doc_commands[fnc_name] = new_command
            else:
                if re.match(r'\s*Commands:\s*', compose_doc_line):
                    doc_commands = {}

                pre_doc += compose_doc_line + '\n'

        doc_commands = collections.OrderedDict(sorted(doc_commands.items()))
        return pre_doc + '\n'.join(doc_commands.values())

    def wrap(fnc):
        def return_fnc(*args, **kargs):
            raise PluginCommandError(
                "Command function '{}' must not called out of scope.".format(fnc.__name__)
            )

        # Using __modified_doc__ as fix for http://bugs.python.org/issue12773
        if hasattr(compose.cli.main.TopLevelCommand, '__modified_doc__'):
            original_doc = compose.cli.main.TopLevelCommand.__modified_doc__
        else:
            original_doc = compose.cli.main.TopLevelCommand.__doc__

        fnc.__standalone__ = standalone
        modified_doc = update_command_doc(original_doc, fnc.__name__, fnc.__doc__)

        try:
            compose.cli.main.TopLevelCommand.__doc__ = modified_doc
        except AttributeError:
            compose.cli.main.TopLevelCommand.__modified_doc__ = modified_doc

        setattr(compose.cli.main.TopLevelCommand, fnc.__name__, fnc)
        return return_fnc
    return wrap


class PluginError(Exception):
    pass


class PluginJsonFileError(PluginError):
    pass


class PluginNotImplementError(PluginError):
    pass


class PluginCommandError(PluginError):
    pass


class Plugin:
    required_fields = ['name', 'version']

    def __init__(self, plugin_manager, config):
        self.plugin_manager = plugin_manager
        self.config = config
        file = os.path.abspath(inspect.getfile(self.__class__))
        self.path = os.path.dirname(file)
        self.id = os.path.basename(self.path)
        self.name = self.id
        self.description = ''
        self.version = None
        self.config = None

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
        return True

    def uninstall(self):
        return True

    def update(self, old_version):
        return None

    def configure(self):
        return None
