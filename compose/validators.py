from cerberus import Validator
from cerberus.errors import ERROR_BAD_TYPE, ERROR_UNALLOWED_VALUE
import os
from re import compile as re_compile
import six

ptrn_cap = '^(?!CAP_)[A-Z]+(_[A-Z]+)*$'
ptrn_container_name = '[a-z0-9_-]+'
ptrn_domain = '[a-z0-9-]+(\.[a-z0-9-]+)+'
ptrn_image = '^([a-z0-9_-]+/)?[a-z0-9_-]+(:[a-z0-9._-]+)?$'
ptrn_hostname = '[a-z0-9-]+'
ptrn_ip4 = '(([0-9]{1,3})\.){3}[0-9]{1,3}'
ptrn_ip6 = '([0-9a-f]{4}:){7}[0-9a-f]{4}'
ptrn_label = '[a-z0-9._-]+'  # TODO exclude two consecutive dashs and dots
ptrn_net = '^(bridge|none|host|container:' + ptrn_container_name + ')$'
ptrn_port = '^[0-9]{0,4}$'
ptrn_security_opts = '^(label:((user|role|type|level).*|disable)|apparmor:.*)'
ptrn_service_name = '^[a-zA-Z0-9]+$'
ptrn_url = '^(https?://|git(@|://|hub\.com/)).*$'
ptrn_ip = '(' + ptrn_ip4 + '|' + ptrn_ip6 + ')'
ptrn_extra_host = '^(' + ptrn_hostname + '|' + ptrn_domain + '):' + ptrn_ip + '$'  # noqa https://github.com/docker/compose/issues/1422
ptrn_extra_host = '^(' + ptrn_hostname + '|' + ptrn_domain + '):' + ptrn_ip4 + '$'  # TODO remove when  ^solved^

re_container_name = re_compile('^' + ptrn_container_name + '$')
re_container_alias_mapping = re_compile('^' + ptrn_container_name + ':' + ptrn_container_name + '$')
re_ip = re_compile('^' + ptrn_ip + '$')
re_port = re_compile(ptrn_port)
re_service_name = re_compile(ptrn_service_name)
re_url = re_compile(ptrn_url)

capabilities = {'type': ['string', 'list'], 'regex': ptrn_cap, 'schema': {'type': 'string', 'regex': ptrn_cap}}
memory = {'type': ['integer', 'string'], 'regex': '^[1-9][0-9]*(b|k|m|g)?$', 'min': 1}
string_or_stringlist = {'type': ['string', 'list'], 'schema': {'type': 'string'}}

env_schema = {'environment': {'type': 'dict', 'valueschema': {'type': 'string', 'nullable': True}}}

file_schema = {'file': {'type': 'dict',
                        'propertyschema': {'type': 'string', 'regex': ptrn_service_name},
                        'valueschema': {'type': 'dict'}}}

service_schema = {'build': {'type': 'buildpath'},
                  'cap_add': capabilities,
                  'cap_drop': capabilities,
                  'cpu_shares': {'type': 'integer', 'min': 0, 'max': 1024},
                  'cpuset': {'type': 'string', 'regex': '^([0-9]+|[0-9]+-[0-9]+)(,([0-9]+|[0-9]+-[0-9]+))?'},
                  'command': string_or_stringlist,
                  'devices': {'type': 'list', 'schema': {'type': 'devicemapping'}},
                  'dns': {'type': ['ip', 'list'], 'schema': {'type': 'ip'}},
                  'dns_search': {'type': ['string', 'list'], 'regex': '^' + ptrn_domain + '$',
                                 'schema': {'type': 'string', 'regex': '^' + ptrn_domain + '$'}},
                  'domainname': {'type': 'string', 'regex': '^' + ptrn_domain + '$'},
                  'entrypoint': string_or_stringlist,
                  'environment': {'type': 'dict', 'valueschema': {'type': 'string', 'nullable': True}},
                  'expose': {'type': 'list', 'schema': {'type': 'port'}},
                  'external_links': {'type': 'list', 'schema': {'type': ['container', 'container_alias_mapping']}},
                  'extra_hosts': {'type': ['string', 'list', 'dict'],  # DRY?!?
                                  'regex': ptrn_extra_host,  # string
                                  'schema': {'type': ['string', 'dict'],  # list
                                             'regex': ptrn_extra_host,  # string in list
                                             'propertyschema': {'type': 'string', 'regex': '^(' + ptrn_hostname + '|' + ptrn_domain + ')$'},  # dict in list
                                             'valueschema': {'type': 'string', 'regex': '^' + ptrn_ip + '$'}},  # dict in list
                                  'propertyschema': {'type': 'string', 'regex': '^(' + ptrn_hostname + '|' + ptrn_domain + ')$'},  # dict
                                  'valueschema': {'type': 'string', 'regex': '^' + ptrn_ip + '$'}},  # dict
                  'hostname': {'type': 'string', 'regex': '^' + ptrn_hostname + '$'},
                  'image': {'type': 'string', 'regex': ptrn_image},
                  'labels': {'type': ['dict', 'list'],
                             'schema': {'type': 'string', 'regex': '^' + ptrn_label + '(=.+)?$'},  # list
                             'propertyschema': {'type': 'string', 'regex': '^' + ptrn_label + '$'},  # dict
                             'valueschema': {'type': 'string', 'nullable': True}},  # dict
                  'links': {'type': 'list', 'schema': {'type': ['container_alias_mapping', 'service_name']}},
                  'log_driver': {'type': 'string', 'regex': '^(json-file|none|syslog)$'},
                  'mac_address': {'type': 'string', 'regex': '^[0-9a-f]{2}(:[0-9a-f]{2}){5}$'},
                  'mem_limit': memory,
                  'memswap_limit': memory,
                  'name': {'type': 'string', 'regex': ptrn_service_name},
                  'net': {'type': 'string', 'regex': ptrn_net},
                  'pid': {'type': 'string', 'nullable': True, 'regex': '^host$'},
                  'ports': {'type': 'list', 'schema': {'type': ['port', 'portmapping']}},
                  'privileged': {'type': 'boolean'},
                  'read_only': {'type': 'boolean'},
                  'restart': {'type': 'string', 'regex': '^(no|on-failure(:[0-9]+)?|always)$'},
                  'security_opt': {'type': 'list', 'schema': {'type': 'string', 'regex': ptrn_security_opts}},
                  'stdin_open': {'type': 'boolean'},
                  'tty': {'type': 'boolean'},
                  'user': {'type': 'string', 'regex': '[a-z_][a-z0-9_-]*[$]?', 'maxlength': 32},  # man 8 useradd
                  'volumes': {'type': 'list', 'schema': {'type': 'volume'}},
                  'volumes_from': {'type': 'list', 'schema': {'type': ['service_name', 'container']}},
                  'working_dir': {'type': 'string'},
                  }

ERROR_NO_DIR = "`%s` is not a directory"
ERROR_UNACCESSIBLE_PATH = "`%s` is not accessible"


class FileValidator(Validator):
    def __init__(self, schema=file_schema, allow_unknown=False, **kwargs):
        self.filename = kwargs.get('filename')
        super(FileValidator, self).__init__(schema, allow_unknown, **kwargs)  # TODO pass prefix when trail is implemented


class ServiceValidator(Validator):
    def __init__(self, schema=service_schema, allow_unknown=False, **kwargs):
        error_source_prefix = kwargs.get('service_name')
        if error_source_prefix is not None:
            error_source_prefix += '/'
        self.working_dir = kwargs.get('working_dir')
        super(ServiceValidator, self).__init__(schema, allow_unknown, **kwargs)  # TODO pass prefix when trail is implemented

    def validate(self, document, schema=None, update=False, context=None, is_extended=False):
        super(ServiceValidator, self).validate(document, schema, update, context)

        if context is None and not is_extended:
            if 'build' in document and 'image' in document:
                self._error(document['name'], 'provide *either* `build` or `image`')
            if not ('build' in document or 'image' in document):
                self._error(document['name'], 'a `build`-path or `image` must be provided')

        return not self.errors

    def _validate_type_accessible_directory(self, field, value):
        if not isinstance(value, six.string_types):
            self._error(field, ERROR_BAD_TYPE % 'string')
        else:
            if self.working_dir:
                value = os.path.abspath(os.path.join(self.working_dir, value))
            if not os.path.isdir(value):
                self._error(field, ERROR_NO_DIR % value)
            elif not os.access(value, os.R_OK | os.X_OK):
                self._error(field, ERROR_UNACCESSIBLE_PATH % value)
            elif field == 'build':
                try:
                    del self.schema['build']['regex']
                except KeyError:
                    pass

    def _validate_type_accessible_path(self, field, value):
        if not isinstance(value, six.string_types):
            self._error(field, ERROR_BAD_TYPE % 'string')
        elif not value:
            self._error(field, ERROR_UNALLOWED_VALUE.format('empty string'))
        else:
            if self.working_dir:
                value = os.path.abspath(os.path.join(self.working_dir, value))
            if not os.access(value, os.R_OK):
                self._error(field, ERROR_UNACCESSIBLE_PATH % value)

    def _validate_type_buildpath(self, field, value):
        prev_errors = self._errors.copy()
        self._validate_type_accessible_directory(field, value)
        if len(self._errors) > len(prev_errors):
            if isinstance(value, six.string_types) and re_url.match(value):
                self._errors = prev_errors
            else:
                self._error(field, 'is no proper URL')

    def _validate_type_container(self, field, value):
        """
        the following imports are done
          - at this point due to cross-imports
          - at all because there are tests that use such objects instead of strings
            - if this direct usage of objects isn't needed in live-usage
              - the tests should be refactored
              - then this code can be removed
        """
        from .container import Container
        from .service import Service

        if isinstance(value, six.string_types):
            if not re_container_name.match(value):
                self._error(field, ERROR_UNALLOWED_VALUE % value)
        elif not isinstance(value, (Service, Container)):
            self._error(field, ERROR_BAD_TYPE % 'string, Container- or Service-instance')

    def _validate_type_container_alias_mapping(self, field, value):
        """
        the following import is done
          - at this point due to cross-imports
          - at all because there are tests that use such objects instead of strings
            - if this direct usage of objects isn't needed in live-usage
              - the tests should be refactored
              - then this code can be removed
        """
        from .service import Service

        if isinstance(value, six.string_types):
            if not re_container_alias_mapping.match(value):
                self._error(field, ERROR_UNALLOWED_VALUE % value)
        elif isinstance(value, tuple):
            if len(value) != 2:
                self._error(field, 'tuple must contain two values')
            else:
                if not isinstance(value[0], Service):
                    self._error(field, ERROR_BAD_TYPE % 'Service-instance')

                if isinstance(value[1], six.string_types):
                    self._validate_type_container(field, value[1])
                elif not isinstance(value[1], (Service, type(None))):
                    self._error(field, ERROR_UNALLOWED_VALUE % value[1])
        else:
            self._error(field, ERROR_BAD_TYPE % 'string or tuple')

    def _validate_type_devicemapping(self, field, value):
        if not isinstance(value, six.string_types):
            self._error(field, ERROR_BAD_TYPE % 'string')

        tokens = value.split(':')
        if not 1 < len(tokens) < 3:
            self._error(field, ERROR_UNALLOWED_VALUE % value)
        else:
            if not (tokens[0].startswith('/dev/') and tokens[1].startswith('/dev/')):
                self._error(field, ERROR_UNALLOWED_VALUE % '; device-path must begin with `/dev/`')
            if len(tokens) == 3:
                for char in tokens[2]:
                    if char not in ['m', 'r', 'w']:
                        self._error(ERROR_UNALLOWED_VALUE % char)

    def _validate_type_ip(self, field, value):
        if not isinstance(value, six.string_types):
            self._error(field, ERROR_BAD_TYPE % 'string')
            return
        if not re_ip.match(value):
            self._error(field, ERROR_UNALLOWED_VALUE % value)
        elif '.' in value:
            for i in value.split('.'):
                if not 0 <= int(i) <= 255:
                    self._error(field, ERROR_UNALLOWED_VALUE % value)
                    break

    def _validate_type_port(self, field, value):
        if isinstance(value, six.string_types) and '-' in value:
            self._validate_type_port(field, value.split('-')[0])
            self._validate_type_port(field, value.split('-')[1])
            return
        try:
            if not 0 < int(value) < 65535:
                self._error(field, ERROR_UNALLOWED_VALUE.format(value))
        except ValueError:
            self._error(field, ERROR_UNALLOWED_VALUE.format(value))

    def _validate_type_portmapping(self, field, value):

        def validate_port(value):
            if value.endswith(('/tcp', '/udp')):
                value = value[:-4]
            self._validate_type_port(field, value)

        def validate_range_match(a, b):
            if int(a.split('-')[1]) - int(a.split('-')[0]) != int(b.split('-')[1]) - int(b.split('-')[0]):
                self._error(field, 'port ranges do not match')

        if isinstance(value, six.string_types):
            if value.startswith('['):  # IPv6
                sliced = value.split(']')
                tokens = [sliced[0][1:]]
                tokens.extend(sliced[1].split(':')[1:])
            else:
                tokens = value.split(':')

            if len(tokens) == 1:
                validate_port(tokens[0])
            elif len(tokens) == 2:
                validate_port(tokens[0])
                validate_port(tokens[1])
                if '-' in tokens[0]:
                    validate_range_match(tokens[0], tokens[1])
            elif len(tokens) == 3:
                self._validate_type_ip(field, tokens[0])
                validate_port(tokens[1])
                validate_port(tokens[2])
                if '-' in tokens[1]:
                    validate_range_match(tokens[1], tokens[2])
            else:
                self._error(field, ERROR_UNALLOWED_VALUE % value)
        else:
            self._error(field, ERROR_BAD_TYPE % 'string or integer')

    def _validate_type_service_name(self, field, value):
        """
        This is solely implemented, because in `service_schema['links']` a 'regex'-declaration would fail
        in case of a `container` or `container_alias_mapping`.
        In order to get rid of it, cerberus.Validator shouldn't check regex-tests if type is not string,
        but that may quiet be a pita.
        """
        # FIXME this is obsolete, code can be removed in favor of a regex-rule
        if isinstance(value, six.string_types):
            if not re_service_name.match(value):
                self._error(field, ERROR_UNALLOWED_VALUE % value)
        else:
            self._error(field, ERROR_BAD_TYPE % 'string')

    def _validate_type_volume(self, field, value):
        # TODO allow escaped colons; never figured out how to deal with backslashes in Python-strings
        if not isinstance(value, six.string_types):
            self._error(field, ERROR_BAD_TYPE % 'string')
        else:
            tokens = value.split(':')
            if len(tokens) == 1:
                pass
            elif len(tokens) == 2:
                self._validate_type_accessible_path(field, tokens[0])
            elif len(tokens) == 3:
                self._validate_type_accessible_path(field, tokens[0])
                if tokens[2] not in ['ro', 'rw']:
                    self._error(field, 'only `:ro` and `:rw` are allowed as suffix')
            else:
                    self._error(field, '%s splits by more than two colons' % value)


class EnvironmentValidator(Validator):
    def __init__(self, schema=env_schema, allow_unknown=False):
        super(EnvironmentValidator, self).__init__(schema, allow_unknown)


# TODO prettify error message, include proposals for invalid keys -> https://github.com/nicolaiarocci/cerberus/issues/93
# TODO review service.py again
# TODO update error-messages for PEP3101-compliance
