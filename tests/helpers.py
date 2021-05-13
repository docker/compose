import contextlib
import os

from compose.config.config import ConfigDetails
from compose.config.config import ConfigFile
from compose.config.config import load

BUSYBOX_IMAGE_NAME = 'busybox'
BUSYBOX_DEFAULT_TAG = '1.31.0-uclibc'
BUSYBOX_IMAGE_WITH_TAG = '{}:{}'.format(BUSYBOX_IMAGE_NAME, BUSYBOX_DEFAULT_TAG)


def build_config(contents, **kwargs):
    return load(build_config_details(contents, **kwargs))


def build_config_details(contents, working_dir='working_dir', filename='filename.yml'):
    return ConfigDetails(
        working_dir,
        [ConfigFile(filename, contents)],
    )


def create_custom_host_file(client, filename, content):
    dirname = os.path.dirname(filename)
    container = client.create_container(
        BUSYBOX_IMAGE_WITH_TAG,
        ['sh', '-c', 'echo -n "{}" > {}'.format(content, filename)],
        volumes={dirname: {}},
        host_config=client.create_host_config(
            binds={dirname: {'bind': dirname, 'ro': False}},
            network_mode='none',
        ),
    )
    try:
        client.start(container)
        exitcode = client.wait(container)['StatusCode']

        if exitcode != 0:
            output = client.logs(container)
            raise Exception(
                "Container exited with code {}:\n{}".format(exitcode, output))

        container_info = client.inspect_container(container)
        if 'Node' in container_info:
            return container_info['Node']['Name']
    finally:
        client.remove_container(container, force=True)


def create_host_file(client, filename):
    with open(filename) as fh:
        content = fh.read()

    return create_custom_host_file(client, filename, content)


@contextlib.contextmanager
def cd(path):
    """
    A context manager which changes the working directory to the given
    path, and then changes it back to its previous value on exit.
    """
    prev_cwd = os.getcwd()
    os.chdir(path)
    try:
        yield
    finally:
        os.chdir(prev_cwd)
