from __future__ import absolute_import
from __future__ import unicode_literals
import copy
import errno

import six

import yaml

from fig.cli.errors import FigFileNotFound

from fig.service import ConfigError


class TemplateLoader(object):
    def __init__(self):
        self.yml_cache = {}

    def load(self, config_path):
        '''
        Iterate over each service to see if they reference a template. If so,
        use all the values, then update the data with the given config.

        A template can refer to a string, which is the filename of the template
        with the same service name.

        A template can also refer to a dict which can reference a "path" and
        "service".
        '''
        config = self.yml_cache.get(config_path)

        if not config:
            # Exceptions are caught outside of this method, in command.py and
            # update_merged_template.
            with open(config_path, 'r') as fh:
                config = yaml.safe_load(fh)
            self.yml_cache[config_path] = config

        for service, data in six.iteritems(config):
            template = data.get('template')
            if not template:
                continue

            # Strip off the template key since we've handled it, and causes
            # recursion issues if left in.
            del data['template']

            tpl_path, tpl_service = TemplateLoader.get_template_values(
                template, service, config_path
            )

            self.update_merged_template(config, service, tpl_path, tpl_service)

        return config

    @staticmethod
    def get_template_values(value, name, config_path):
        '''
        Parse the template value into a template path and template service.

        This will use the defaults specified if the values are empty, i.e.
        name and config_path.

        >>> TemplateLoader.get_template_values({}, 'service', 'fig.yml')
        ... 5
        '''
        if isinstance(value, six.string_types):
            path = value
            service = name
        elif isinstance(value, dict):
            path = value.get('path')
            service = value.get('service')
            if not path and not service:
                raise ConfigError('Template can not be empty, otherwise it '
                                  'would reference itself!')

        else:
            raise ConfigError('Template is not a string or dict')

        service = service or name
        path = path or config_path
        return path, service

    def update_merged_template(self, config, name, tpl_path, tpl_service):
        '''
        Updates "config" in-place with a template based service.
        '''
        try:
            tpl_config = self.load(tpl_path)
        except IOError as e:
            if e.errno == errno.ENOENT:
                raise FigFileNotFound('Template not found: %s' % tpl_path)
            raise e  # Let command.py get_config.py handle unexpected IOErrors

        if tpl_service not in tpl_config:
            raise ConfigError(
                'Service "%s" referenced in "%s" does not exist' %
                (tpl_service, tpl_path)
            )

        service = copy.copy(tpl_config[tpl_service])
        service.update(config.get(name))
        config[name] = service
