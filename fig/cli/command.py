from __future__ import unicode_literals
from __future__ import absolute_import
from docker import Client
import errno
import logging
import os
import re
import yaml

from ..project import Project
from .docopt_command import DocoptCommand
from .formatter import Formatter
from .utils import cached_property, docker_url

log = logging.getLogger(__name__)

class Command(DocoptCommand):
    base_dir = '.'

    @cached_property
    def client(self):
        return Client(docker_url())

    @cached_property
    def project(self):
        try:
            yaml_path = os.path.join(self.base_dir, 'fig.yml')
            config = yaml.load(open(yaml_path))
        except IOError as e:
            if e.errno == errno.ENOENT:
                log.error("Can't find %s. Are you in the right directory?", os.path.basename(e.filename))
            else:
                log.error(e)

            exit(1)

        return Project.from_config(self.project_name, config, self.client)

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

