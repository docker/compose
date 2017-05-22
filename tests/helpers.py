from __future__ import absolute_import
from __future__ import unicode_literals

import functools
import os

from docker.errors import APIError
from pytest import skip

from compose.config.config import ConfigDetails
from compose.config.config import ConfigFile
from compose.config.config import load


def build_config(contents, **kwargs):
    return load(build_config_details(contents, **kwargs))


def build_config_details(contents, working_dir='working_dir', filename='filename.yml'):
    return ConfigDetails(
        working_dir,
        [ConfigFile(filename, contents)],
    )


def create_host_file(client, filename):
    dirname = os.path.dirname(filename)

    with open(filename, 'r') as fh:
        content = fh.read()

    container = client.create_container(
        'busybox:latest',
        ['sh', '-c', 'echo -n "{}" > {}'.format(content, filename)],
        volumes={dirname: {}},
        host_config=client.create_host_config(
            binds={dirname: {'bind': dirname, 'ro': False}},
            network_mode='none',
        ),
    )
    try:
        client.start(container)
        exitcode = client.wait(container)

        if exitcode != 0:
            output = client.logs(container)
            raise Exception(
                "Container exited with code {}:\n{}".format(exitcode, output))
    finally:
        client.remove_container(container, force=True)


def is_cluster(client):
    nodes = None

    def get_nodes_number():
        try:
            return len(client.nodes())
        except APIError:
            # If the Engine is not part of a Swarm, the SDK will raise
            # an APIError
            return 0

    if nodes is None:
        # Only make the API call if the value hasn't been cached yet
        nodes = get_nodes_number()

    return nodes > 1


def no_cluster(reason):
    def decorator(f):
        @functools.wraps(f)
        def wrapper(self, *args, **kwargs):
            if is_cluster(self.client):
                skip("Test will not be run in cluster mode: %s" % reason)
                return
            return f(self, *args, **kwargs)
        return wrapper

    return decorator
