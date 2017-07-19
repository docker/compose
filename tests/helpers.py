from __future__ import absolute_import
from __future__ import unicode_literals

import os

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
