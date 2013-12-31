from docker import Client
import logging
import os
import re
import yaml
import socket

from ..project import Project
from .docopt_command import DocoptCommand
from .formatter import Formatter
from .utils import cached_property
from .errors import UserError

log = logging.getLogger(__name__)

class Command(DocoptCommand):
    @cached_property
    def client(self):
        if os.environ.get('DOCKER_URL'):
            return Client(os.environ['DOCKER_URL'])

        socket_path = '/var/run/docker.sock'
        tcp_host = '127.0.0.1'
        tcp_port = 4243

        if os.path.exists(socket_path):
            return Client('unix://%s' % socket_path)

        try:
            s = socket.socket()
            s.connect((tcp_host, tcp_port))
            s.close()
            return Client('http://%s:%s' % (tcp_host, tcp_port))
        except:
            pass

        raise UserError("""
        Couldn't find Docker daemon - tried %s and %s:%s.
        If it's running elsewhere, specify a url with DOCKER_URL.
        """ % (socket_path, tcp_host, tcp_port))

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

