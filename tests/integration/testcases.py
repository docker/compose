from __future__ import absolute_import
from __future__ import unicode_literals

import hashlib
import logging

from .. import unittest
from compose.cli.docker_client import docker_client
from compose.config.config import ServiceLoader
from compose.const import LABEL_PROJECT
from compose.progress_stream import stream_output
from compose.service import Service


log = logging.getLogger(__name__)


LABEL_TEST_IMAGE = 'com.docker.compose.test-image'


class DockerClientTestCase(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        cls.client = docker_client()

    def tearDown(self):
        project_label = '%s=%s' % (LABEL_PROJECT, self.project_name)
        for c in self.client.containers(
                all=True,
                filters={'label': project_label}):
            self.client.kill(c['Id'])
            self.client.remove_container(c['Id'])
        for i in self.client.images(
                filters={'label': LABEL_TEST_IMAGE}):
            try:
                self.client.remove_image(i)
            except Exception as e:
                log.warn("Failed to remove %s: %s" % (i, e))

    def create_service(self, name, **kwargs):
        if 'image' not in kwargs and 'build' not in kwargs:
            kwargs['image'] = 'busybox:latest'

        kwargs.setdefault('command', ["top"])

        links = kwargs.get('links', None)
        volumes_from = kwargs.get('volumes_from', None)
        net = kwargs.get('net', None)

        workaround_options = ['links', 'volumes_from', 'net']
        for key in workaround_options:
            try:
                del kwargs[key]
            except KeyError:
                pass

        options = ServiceLoader(
            working_dir='.',
            filename=None,
            service_name=name,
            service_dict=kwargs).make_service_dict()

        labels = options.setdefault('labels', {})
        labels['com.docker.compose.test-name'] = self.id()

        if links:
            options['links'] = links
        if volumes_from:
            options['volumes_from'] = volumes_from
        if net:
            options['net'] = net

        return Service(
            project=self.project_name,
            client=self.client,
            **options
        )

    @property
    def project_name(self):
        hash = hashlib.new('md5')
        hash.update(self.id().encode('utf-8'))
        return 'ct' + hash.hexdigest()

    def check_build(self, *args, **kwargs):
        kwargs.setdefault('rm', True)
        build_output = self.client.build(*args, **kwargs)
        stream_output(build_output, open('/dev/null', 'w'))
