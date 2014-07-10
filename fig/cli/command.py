from __future__ import unicode_literals
from __future__ import absolute_import
from ..packages.docker import Client
from requests.exceptions import ConnectionError
import errno
import logging
import os
import re
import yaml
from ..packages import six
import sys

from ..project import Project
from ..service import ConfigError
from .docopt_command import DocoptCommand
from .formatter import Formatter
from .utils import cached_property, docker_url, call_silently, is_mac, is_ubuntu
from . import errors

log = logging.getLogger(__name__)

class Command(DocoptCommand):
    base_dir = '.'

    def __init__(self):
        self.yaml_path = os.environ.get('FIG_FILE', None)
        self.explicit_project_name = None

    def dispatch(self, *args, **kwargs):
        try:
            super(Command, self).dispatch(*args, **kwargs)
        except ConnectionError:
            if call_silently(['which', 'docker']) != 0:
                if is_mac():
                    raise errors.DockerNotFoundMac()
                elif is_ubuntu():
                    raise errors.DockerNotFoundUbuntu()
                else:
                    raise errors.DockerNotFoundGeneric()
            elif call_silently(['which', 'docker-osx']) == 0:
                raise errors.ConnectionErrorDockerOSX()
            else:
                raise errors.ConnectionErrorGeneric(self.client.base_url)

    def perform_command(self, options, *args, **kwargs):
        if options['--file'] is not None:
            self.yaml_path = os.path.join(self.base_dir, options['--file'])
        if options['--project-name'] is not None:
            self.explicit_project_name = options['--project-name']
        return super(Command, self).perform_command(options, *args, **kwargs)

    @cached_property
    def client(self):
        return Client(docker_url())

    @cached_property
    def project(self):
        try:
            yaml_path = self.yaml_path
            if yaml_path is None:
                yaml_path = self.check_yaml_filename()
            config = yaml.safe_load(open(yaml_path))
        except IOError as e:
            if e.errno == errno.ENOENT:
                raise errors.FigFileNotFound(os.path.basename(e.filename))
            raise errors.UserError(six.text_type(e))

        try:
            return Project.from_config(self.project_name, config, self.client)
        except ConfigError as e:
            raise errors.UserError(six.text_type(e))

    @cached_property
    def project_name(self):
        project = os.path.basename(os.getcwd())
        if self.explicit_project_name is not None:
            project = self.explicit_project_name
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
