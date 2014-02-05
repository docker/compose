from __future__ import unicode_literals
from __future__ import absolute_import
from ..packages.docker import Client
from requests.exceptions import ConnectionError
import errno
import logging
import os
import re
import yaml
import six

from ..project import Project
from ..service import ConfigError
from .docopt_command import DocoptCommand
from .formatter import Formatter
from .utils import cached_property, docker_url, call_silently, is_mac, is_ubuntu
from .errors import UserError

log = logging.getLogger(__name__)

class Command(DocoptCommand):
    base_dir = '.'

    def dispatch(self, *args, **kwargs):
        try:
            super(Command, self).dispatch(*args, **kwargs)
        except ConnectionError:
            if call_silently(['which', 'docker']) != 0:
                if is_mac():
                    raise UserError("""
Couldn't connect to Docker daemon. You might need to install docker-osx:

https://github.com/noplay/docker-osx
""")
                elif is_ubuntu():
                    raise UserError("""
Couldn't connect to Docker daemon. You might need to install Docker:

http://docs.docker.io/en/latest/installation/ubuntulinux/
""")
                else:
                    raise UserError("""
Couldn't connect to Docker daemon. You might need to install Docker:

http://docs.docker.io/en/latest/installation/
""")
            elif call_silently(['which', 'docker-osx']) == 0:
                raise UserError("Couldn't connect to Docker daemon - you might need to run `docker-osx shell`.")
            else:
                raise UserError("""
Couldn't connect to Docker daemon at %s - is it running?

If it's at a non-standard location, specify the URL with the DOCKER_HOST environment variable.
""" % self.client.base_url)

    @cached_property
    def client(self):
        return Client(docker_url())

    @cached_property
    def project(self):
        try:
            yaml_path = self.check_yaml_filename()
            config = yaml.load(open(yaml_path))

        except IOError as e:
            if e.errno == errno.ENOENT:
                log.error("Can't find %s. Are you in the right directory?", os.path.basename(e.filename))
            else:
                log.error(e)

            exit(1)

        try:
            return Project.from_config(self.project_name, config, self.client)
        except ConfigError as e:
            raise UserError(six.text_type(e))

    @cached_property
    def project_name(self):
        project = os.path.basename(os.getcwd())
        project = re.sub(r'[^a-zA-Z0-9]', '', project)
        if not project:
            project = 'default'
        return project

    @cached_property
    def formatter(self):
        return Formatter()

    def check_yaml_filename(self):
        if os.path.exists(os.path.join(self.base_dir, 'fig.yaml')):

            log.warning("Fig just read the file 'fig.yaml' on startup, rather than 'fig.yml'")
            log.warning("Please be aware that fig.yml the expected extension in most cases, and using .yaml can cause compatibility issues in future")

            return os.path.join(self.base_dir, 'fig.yaml')
        else:
            return os.path.join(self.base_dir, 'fig.yml')
