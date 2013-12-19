from docker import Client
import logging
import os
import re
import yaml

from ..service_collection import ServiceCollection
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
    def service_collection(self):
        config = yaml.load(open('plum.yml'))
        return ServiceCollection.from_config(
            config,
            client=self.client,
            project=self.project
        )

    @cached_property
    def project(self):
        project = os.path.basename(os.getcwd())
        project = re.sub(r'[^a-zA-Z0-9]', '', project)
        if not project:
            project = 'default'
        return project

    @cached_property
    def formatter(self):
        return Formatter()

