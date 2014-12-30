from __future__ import absolute_import
from __future__ import unicode_literals

import six

import yaml

from . import errors


def load(config_path):
    '''
    Iterate over each service to see if they reference a template. If so,
    use all the values, then update the data with the given config.

    A template can refer to a string, which is the filename of the template
    with the same service name.

    A template can also refer to a dict which can reference a "path" and
    "service".
    '''
    with open(config_path, 'r') as fh:
        config = yaml.safe_load(fh)

    for name, data in six.iteritems(config):
        template = data.get('template')
        if not template:
            continue

        # Strip off the template key since we don't need it now
        del data['template']

        tpl_path, tpl_service = get_template_values(name, template)

        update_merged_template(config, name, tpl_path, tpl_service)

    return config


def get_template_values(name, value):
    if isinstance(value, six.string_types):
        path = value
        service = name
    elif isinstance(value, dict):
        path = value.get('path')
        service = value.get('service')
    else:
        raise errors.UserError(six.text_type('template is not a string'
                                             ' or dict'))
    return path, service


def update_merged_template(config, name, tpl_path, tpl_service):
    '''
    Updates "config" in-place with a template based service.
    '''
    tpl_config = load(tpl_path)
    service = tpl_config[tpl_service]
    service.update(config.get(name))
    config[name] = service
