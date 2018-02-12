from __future__ import absolute_import
from __future__ import unicode_literals

from functools import reduce

import six
from docker.errors import ImageNotFound

from .const import LABEL_CONTAINER_NUMBER
from .const import LABEL_PROJECT
from .const import LABEL_SERVICE


class Container(object):
    """
    Represents a Docker container, constructed from the output of
    GET /containers/:id:/json.
    """
    def __init__(self, client, dictionary, has_been_inspected=False):
        self.client = client
        self.dictionary = dictionary
        self.has_been_inspected = has_been_inspected
        self.log_stream = None

    @classmethod
    def from_ps(cls, client, dictionary, **kwargs):
        """
        Construct a container object from the output of GET /containers/json.
        """
        name = get_container_name(dictionary)
        if name is None:
            return None

        new_dictionary = {
            'Id': dictionary['Id'],
            'Image': dictionary['Image'],
            'Name': '/' + name,
        }
        return cls(client, new_dictionary, **kwargs)

    @classmethod
    def from_id(cls, client, id):
        return cls(client, client.inspect_container(id), has_been_inspected=True)

    @classmethod
    def create(cls, client, **options):
        response = client.create_container(**options)
        return cls.from_id(client, response['Id'])

    @property
    def id(self):
        return self.dictionary['Id']

    @property
    def image(self):
        return self.dictionary['Image']

    @property
    def image_config(self):
        return self.client.inspect_image(self.image)

    @property
    def short_id(self):
        return self.id[:12]

    @property
    def name(self):
        return self.dictionary['Name'][1:]

    @property
    def project(self):
        return self.labels.get(LABEL_PROJECT)

    @property
    def service(self):
        return self.labels.get(LABEL_SERVICE)

    @property
    def name_without_project(self):
        if self.name.startswith('{0}_{1}'.format(self.project, self.service)):
            return '{0}_{1}'.format(self.service, self.number)
        else:
            return self.name

    @property
    def number(self):
        number = self.labels.get(LABEL_CONTAINER_NUMBER)
        if not number:
            raise ValueError("Container {0} does not have a {1} label".format(
                self.short_id, LABEL_CONTAINER_NUMBER))
        return int(number)

    @property
    def ports(self):
        self.inspect_if_not_inspected()
        return self.get('NetworkSettings.Ports') or {}

    @property
    def human_readable_ports(self):
        def format_port(private, public):
            if not public:
                return [private]
            return [
                '{HostIp}:{HostPort}->{private}'.format(private=private, **pub)
                for pub in public
            ]

        return ', '.join(
            ','.join(format_port(*item))
            for item in sorted(six.iteritems(self.ports))
        )

    @property
    def labels(self):
        return self.get('Config.Labels') or {}

    @property
    def stop_signal(self):
        return self.get('Config.StopSignal')

    @property
    def log_config(self):
        return self.get('HostConfig.LogConfig') or None

    @property
    def human_readable_state(self):
        if self.is_paused:
            return 'Paused'
        if self.is_restarting:
            return 'Restarting'
        if self.is_running:
            return 'Ghost' if self.get('State.Ghost') else self.human_readable_health_status
        else:
            return 'Exit %s' % self.get('State.ExitCode')

    @property
    def human_readable_command(self):
        entrypoint = self.get('Config.Entrypoint') or []
        cmd = self.get('Config.Cmd') or []
        return ' '.join(entrypoint + cmd)

    @property
    def environment(self):
        def parse_env(var):
            if '=' in var:
                return var.split("=", 1)
            return var, None
        return dict(parse_env(var) for var in self.get('Config.Env') or [])

    @property
    def exit_code(self):
        return self.get('State.ExitCode')

    @property
    def is_running(self):
        return self.get('State.Running')

    @property
    def is_restarting(self):
        return self.get('State.Restarting')

    @property
    def is_paused(self):
        return self.get('State.Paused')

    @property
    def log_driver(self):
        return self.get('HostConfig.LogConfig.Type')

    @property
    def has_api_logs(self):
        log_type = self.log_driver
        return not log_type or log_type in ('json-file', 'journald')

    @property
    def human_readable_health_status(self):
        """ Generate UP status string with up time and health
        """
        status_string = 'Up'
        container_status = self.get('State.Health.Status')
        if container_status == 'starting':
            status_string += ' (health: starting)'
        elif container_status is not None:
            status_string += ' (%s)' % container_status
        return status_string

    def attach_log_stream(self):
        """A log stream can only be attached if the container uses a json-file
        log driver.
        """
        if self.has_api_logs:
            self.log_stream = self.attach(stdout=True, stderr=True, stream=True)

    def get(self, key):
        """Return a value from the container or None if the value is not set.

        :param key: a string using dotted notation for nested dictionary
                    lookups
        """
        self.inspect_if_not_inspected()

        def get_value(dictionary, key):
            return (dictionary or {}).get(key)

        return reduce(get_value, key.split('.'), self.dictionary)

    def get_local_port(self, port, protocol='tcp'):
        port = self.ports.get("%s/%s" % (port, protocol))
        return "{HostIp}:{HostPort}".format(**port[0]) if port else None

    def get_mount(self, mount_dest):
        for mount in self.get('Mounts'):
            if mount['Destination'] == mount_dest:
                return mount
        return None

    def start(self, **options):
        return self.client.start(self.id, **options)

    def stop(self, **options):
        return self.client.stop(self.id, **options)

    def pause(self, **options):
        return self.client.pause(self.id, **options)

    def unpause(self, **options):
        return self.client.unpause(self.id, **options)

    def kill(self, **options):
        return self.client.kill(self.id, **options)

    def restart(self, **options):
        return self.client.restart(self.id, **options)

    def remove(self, **options):
        return self.client.remove_container(self.id, **options)

    def create_exec(self, command, **options):
        return self.client.exec_create(self.id, command, **options)

    def start_exec(self, exec_id, **options):
        return self.client.exec_start(exec_id, **options)

    def rename_to_tmp_name(self):
        """Rename the container to a hopefully unique temporary container name
        by prepending the short id.
        """
        if not self.name.startswith(self.short_id):
            self.client.rename(
                self.id, '{0}_{1}'.format(self.short_id, self.name)
            )

    def inspect_if_not_inspected(self):
        if not self.has_been_inspected:
            self.inspect()

    def wait(self):
        return self.client.wait(self.id).get('StatusCode', 127)

    def logs(self, *args, **kwargs):
        return self.client.logs(self.id, *args, **kwargs)

    def inspect(self):
        self.dictionary = self.client.inspect_container(self.id)
        self.has_been_inspected = True
        return self.dictionary

    def image_exists(self):
        try:
            self.client.inspect_image(self.image)
        except ImageNotFound:
            return False

        return True

    def reset_image(self, img_id):
        """ If this container's image has been removed, temporarily replace the old image ID
            with `img_id`.
        """
        if not self.image_exists():
            self.dictionary['Image'] = img_id

    def attach(self, *args, **kwargs):
        return self.client.attach(self.id, *args, **kwargs)

    def __repr__(self):
        return '<Container: %s (%s)>' % (self.name, self.id[:6])

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return self.id == other.id

    def __hash__(self):
        return self.id.__hash__()


def get_container_name(container):
    if not container.get('Name') and not container.get('Names'):
        return None
    # inspect
    if 'Name' in container:
        return container['Name']
    # ps
    shortest_name = min(container['Names'], key=lambda n: len(n.split('/')))
    return shortest_name.split('/')[-1]
