from docker import Client
import logging
import os
import re
import yaml

from ..project import Project
from .docopt_command import DocoptCommand
from .formatter import Formatter
from .utils import cached_property, mkdir

log = logging.getLogger(__name__)

class Command(DocoptCommand):
    @cached_property
    def client(self):
        if os.environ.get('DOCKER_URL'):
            return Client(os.environ['DOCKER_URL'])
        else:
            return Client()

    @cached_property
    def project(self):
        config = yaml.load(open('fig.yml'))
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

