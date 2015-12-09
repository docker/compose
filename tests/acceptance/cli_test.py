from __future__ import absolute_import

import os
import shlex
import signal
import subprocess
import time
from collections import namedtuple
from operator import attrgetter
from textwrap import dedent

from docker import errors

from .. import mock
from compose.cli.command import get_project
from compose.cli.docker_client import docker_client
from compose.container import Container
from tests.integration.testcases import DockerClientTestCase
from tests.integration.testcases import pull_busybox


ProcessResult = namedtuple('ProcessResult', 'stdout stderr')


BUILD_CACHE_TEXT = 'Using cache'
BUILD_PULL_TEXT = 'Status: Image is up to date for busybox:latest'


def start_process(base_dir, options):
    proc = subprocess.Popen(
        ['docker-compose'] + options,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=base_dir)
    print("Running process: %s" % proc.pid)
    return proc


def wait_on_process(proc, returncode=0):
    stdout, stderr = proc.communicate()
    if proc.returncode != returncode:
        print(stderr.decode('utf-8'))
        assert proc.returncode == returncode
    return ProcessResult(stdout.decode('utf-8'), stderr.decode('utf-8'))


def wait_on_condition(condition, delay=0.1, timeout=20):
    start_time = time.time()
    while not condition():
        if time.time() - start_time > timeout:
            raise AssertionError("Timeout: %s" % condition)
        time.sleep(delay)


def kill_service(service):
    for container in service.containers():
        container.kill()


class ContainerCountCondition(object):

    def __init__(self, project, expected):
        self.project = project
        self.expected = expected

    def __call__(self):
        return len(self.project.containers()) == self.expected

    def __str__(self):
        return "waiting for counter count == %s" % self.expected


class ContainerStateCondition(object):

    def __init__(self, client, name, running):
        self.client = client
        self.name = name
        self.running = running

    # State.Running == true
    def __call__(self):
        try:
            container = self.client.inspect_container(self.name)
            return container['State']['Running'] == self.running
        except errors.APIError:
            return False

    def __str__(self):
        return "waiting for container to have state %s" % self.expected


class CLITestCase(DockerClientTestCase):

    def setUp(self):
        super(CLITestCase, self).setUp()
        self.base_dir = 'tests/fixtures/simple-composefile'

    def tearDown(self):
        if self.base_dir:
            self.project.kill()
            self.project.remove_stopped()
            for container in self.project.containers(stopped=True, one_off=True):
                container.remove(force=True)
        super(CLITestCase, self).tearDown()

    @property
    def project(self):
        # Hack: allow project to be overridden
        if not hasattr(self, '_project'):
            self._project = get_project(self.base_dir)
        return self._project

    def dispatch(self, options, project_options=None, returncode=0):
        project_options = project_options or []
        proc = start_process(self.base_dir, project_options + options)
        return wait_on_process(proc, returncode=returncode)

    def test_help(self):
        self.base_dir = 'tests/fixtures/no-composefile'
        result = self.dispatch(['help', 'up'], returncode=1)
        assert 'Usage: up [options] [SERVICE...]' in result.stderr
        # Prevent tearDown from trying to create a project
        self.base_dir = None

    def test_config_list_services(self):
        result = self.dispatch(['config', '--services'])
        assert set(result.stdout.rstrip().split('\n')) == {'simple', 'another'}

    def test_config_quiet_with_error(self):
        self.base_dir = None
        result = self.dispatch([
            '-f', 'tests/fixtures/invalid-composefile/invalid.yml',
            'config', '-q'
        ], returncode=1)
        assert "'notaservice' doesn't have any configuration" in result.stderr

    def test_config_quiet(self):
        assert self.dispatch(['config', '-q']).stdout == ''

    def test_config_default(self):
        result = self.dispatch(['config'])
        assert dedent("""
            simple:
              command: top
              image: busybox:latest
        """).lstrip() in result.stdout
        assert dedent("""
            another:
              command: top
              image: busybox:latest
        """).lstrip() in result.stdout

    def test_ps(self):
        self.project.get_service('simple').create_container()
        result = self.dispatch(['ps'])
        assert 'simplecomposefile_simple_1' in result.stdout

    def test_ps_default_composefile(self):
        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['up', '-d'])
        result = self.dispatch(['ps'])

        self.assertIn('multiplecomposefiles_simple_1', result.stdout)
        self.assertIn('multiplecomposefiles_another_1', result.stdout)
        self.assertNotIn('multiplecomposefiles_yetanother_1', result.stdout)

    def test_ps_alternate_composefile(self):
        config_path = os.path.abspath(
            'tests/fixtures/multiple-composefiles/compose2.yml')
        self._project = get_project(self.base_dir, [config_path])

        self.base_dir = 'tests/fixtures/multiple-composefiles'
        self.dispatch(['-f', 'compose2.yml', 'up', '-d'])
        result = self.dispatch(['-f', 'compose2.yml', 'ps'])

        self.assertNotIn('multiplecomposefiles_simple_1', result.stdout)
        self.assertNotIn('multiplecomposefiles_another_1', result.stdout)
        self.assertIn('multiplecomposefiles_yetanother_1', result.stdout)

    def test_pull(self):
        result = self.dispatch(['pull'])
        assert sorted(result.stderr.split('\n'))[1:] == [
            'Pulling another (busybox:latest)...',
            'Pulling simple (busybox:latest)...',
        ]

    def test_pull_with_digest(self):
        result = self.dispatch(['-f', 'digest.yml', 'pull'])

        assert 'Pulling simple (busybox:latest)...' in result.stderr
        assert ('Pulling digest (busybox@'
                'sha256:38a203e1986cf79639cfb9b2e1d6e773de84002feea2d4eb006b520'
                '04ee8502d)...') in result.stderr

    def test_pull_with_ignore_pull_failures(self):
        result = self.dispatch([
            '-f', 'ignore-pull-failures.yml',
            'pull', '--ignore-pull-failures'])

        assert 'Pulling simple (busybox:latest)...' in result.stderr
        assert 'Pulling another (nonexisting-image:latest)...' in result.stderr
        assert 'Error: image library/nonexisting-image:latest not found' in result.stderr

    def test_build_plain(self):
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', 'simple'])
        assert BUILD_CACHE_TEXT in result.stdout
        assert BUILD_PULL_TEXT not in result.stdout

    def test_build_no_cache(self):
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', '--no-cache', 'simple'])
        assert BUILD_CACHE_TEXT not in result.stdout
        assert BUILD_PULL_TEXT not in result.stdout

    def test_build_pull(self):
        # Make sure we have the latest busybox already
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'], None)

        result = self.dispatch(['build', '--pull', 'simple'])
        assert BUILD_CACHE_TEXT in result.stdout
        assert BUILD_PULL_TEXT in result.stdout

    def test_build_no_cache_pull(self):
        # Make sure we have the latest busybox already
        pull_busybox(self.client)
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['build', 'simple'])

        result = self.dispatch(['build', '--no-cache', '--pull', 'simple'])
        assert BUILD_CACHE_TEXT not in result.stdout
        assert BUILD_PULL_TEXT in result.stdout

    def test_build_failed(self):
        self.base_dir = 'tests/fixtures/simple-failing-dockerfile'
        self.dispatch(['build', 'simple'], returncode=1)

        labels = ["com.docker.compose.test_failing_image=true"]
        containers = [
            Container.from_ps(self.project.client, c)
            for c in self.project.client.containers(
                all=True,
                filters={"label": labels})
        ]
        assert len(containers) == 1

    def test_build_failed_forcerm(self):
        self.base_dir = 'tests/fixtures/simple-failing-dockerfile'
        self.dispatch(['build', '--force-rm', 'simple'], returncode=1)

        labels = ["com.docker.compose.test_failing_image=true"]

        containers = [
            Container.from_ps(self.project.client, c)
            for c in self.project.client.containers(
                all=True,
                filters={"label": labels})
        ]
        assert not containers

    def test_up_detached(self):
        self.dispatch(['up', '-d'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        self.assertEqual(len(service.containers()), 1)
        self.assertEqual(len(another.containers()), 1)

        # Ensure containers don't have stdin and stdout connected in -d mode
        container, = service.containers()
        self.assertFalse(container.get('Config.AttachStderr'))
        self.assertFalse(container.get('Config.AttachStdout'))
        self.assertFalse(container.get('Config.AttachStdin'))

    def test_up_attached(self):
        self.base_dir = 'tests/fixtures/echo-services'
        result = self.dispatch(['up', '--no-color'])

        assert 'simple_1  | simple' in result.stdout
        assert 'another_1 | another' in result.stdout

    def test_up_without_networking(self):
        self.require_api_version('1.21')

        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d'], None)
        client = docker_client(version='1.21')

        networks = client.networks(names=[self.project.name])
        self.assertEqual(len(networks), 0)

        for service in self.project.get_services():
            containers = service.containers()
            self.assertEqual(len(containers), 1)
            self.assertNotEqual(containers[0].get('Config.Hostname'), service.name)

        web_container = self.project.get_service('web').containers()[0]
        self.assertTrue(web_container.get('HostConfig.Links'))

    def test_up_with_networking(self):
        self.require_api_version('1.21')

        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['--x-networking', 'up', '-d'], None)
        client = docker_client(version='1.21')

        services = self.project.get_services()

        networks = client.networks(names=[self.project.name])
        for n in networks:
            self.addCleanup(client.remove_network, n['Id'])
        self.assertEqual(len(networks), 1)
        self.assertEqual(networks[0]['Driver'], 'bridge')

        network = client.inspect_network(networks[0]['Id'])
        self.assertEqual(len(network['Containers']), len(services))

        for service in services:
            containers = service.containers()
            self.assertEqual(len(containers), 1)
            self.assertIn(containers[0].id, network['Containers'])

        web_container = self.project.get_service('web').containers()[0]
        self.assertFalse(web_container.get('HostConfig.Links'))

    def test_up_with_links(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'web'], None)
        web = self.project.get_service('web')
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

    def test_up_with_no_deps(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', '--no-deps', 'web'], None)
        web = self.project.get_service('web')
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(console.containers()), 0)

    def test_up_with_force_recreate(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.dispatch(['up', '-d', '--force-recreate'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertNotEqual(old_ids, new_ids)

    def test_up_with_no_recreate(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.dispatch(['up', '-d', '--no-recreate'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertEqual(old_ids, new_ids)

    def test_up_with_force_recreate_and_no_recreate(self):
        self.dispatch(
            ['up', '-d', '--force-recreate', '--no-recreate'],
            returncode=1)

    def test_up_with_timeout(self):
        self.dispatch(['up', '-d', '-t', '1'])
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        self.assertEqual(len(service.containers()), 1)
        self.assertEqual(len(another.containers()), 1)

        # Ensure containers don't have stdin and stdout connected in -d mode
        config = service.containers()[0].inspect()['Config']
        self.assertFalse(config['AttachStderr'])
        self.assertFalse(config['AttachStdout'])
        self.assertFalse(config['AttachStdin'])

    def test_up_handles_sigint(self):
        proc = start_process(self.base_dir, ['up', '-t', '2'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        os.kill(proc.pid, signal.SIGINT)
        wait_on_condition(ContainerCountCondition(self.project, 0))

    def test_up_handles_sigterm(self):
        proc = start_process(self.base_dir, ['up', '-t', '2'])
        wait_on_condition(ContainerCountCondition(self.project, 2))

        os.kill(proc.pid, signal.SIGTERM)
        wait_on_condition(ContainerCountCondition(self.project, 0))

    def test_run_service_without_links(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', 'console', '/bin/true'])
        self.assertEqual(len(self.project.containers()), 0)

        # Ensure stdin/out was open
        container = self.project.containers(stopped=True, one_off=True)[0]
        config = container.inspect()['Config']
        self.assertTrue(config['AttachStderr'])
        self.assertTrue(config['AttachStdout'])
        self.assertTrue(config['AttachStdin'])

    def test_run_service_with_links(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', 'web', '/bin/true'], None)
        db = self.project.get_service('db')
        console = self.project.get_service('console')
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

    def test_run_with_no_deps(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['run', '--no-deps', 'web', '/bin/true'])
        db = self.project.get_service('db')
        self.assertEqual(len(db.containers()), 0)

    def test_run_does_not_recreate_linked_containers(self):
        self.base_dir = 'tests/fixtures/links-composefile'
        self.dispatch(['up', '-d', 'db'])
        db = self.project.get_service('db')
        self.assertEqual(len(db.containers()), 1)

        old_ids = [c.id for c in db.containers()]

        self.dispatch(['run', 'web', '/bin/true'], None)
        self.assertEqual(len(db.containers()), 1)

        new_ids = [c.id for c in db.containers()]

        self.assertEqual(old_ids, new_ids)

    def test_run_without_command(self):
        self.base_dir = 'tests/fixtures/commands-composefile'
        self.check_build('tests/fixtures/simple-dockerfile', tag='composetest_test')

        self.dispatch(['run', 'implicit'])
        service = self.project.get_service('implicit')
        containers = service.containers(stopped=True, one_off=True)
        self.assertEqual(
            [c.human_readable_command for c in containers],
            [u'/bin/sh -c echo "success"'],
        )

        self.dispatch(['run', 'explicit'])
        service = self.project.get_service('explicit')
        containers = service.containers(stopped=True, one_off=True)
        self.assertEqual(
            [c.human_readable_command for c in containers],
            [u'/bin/true'],
        )

    def test_run_service_with_entrypoint_overridden(self):
        self.base_dir = 'tests/fixtures/dockerfile_with_entrypoint'
        name = 'service'
        self.dispatch(['run', '--entrypoint', '/bin/echo', name, 'helloworld'])
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        self.assertEqual(
            shlex.split(container.human_readable_command),
            [u'/bin/echo', u'helloworld'],
        )

    def test_run_service_with_user_overridden(self):
        self.base_dir = 'tests/fixtures/user-composefile'
        name = 'service'
        user = 'sshd'
        self.dispatch(['run', '--user={user}'.format(user=user), name], returncode=1)
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        self.assertEqual(user, container.get('Config.User'))

    def test_run_service_with_user_overridden_short_form(self):
        self.base_dir = 'tests/fixtures/user-composefile'
        name = 'service'
        user = 'sshd'
        self.dispatch(['run', '-u', user, name], returncode=1)
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        self.assertEqual(user, container.get('Config.User'))

    def test_run_service_with_environement_overridden(self):
        name = 'service'
        self.base_dir = 'tests/fixtures/environment-composefile'
        self.dispatch([
            'run', '-e', 'foo=notbar',
            '-e', 'allo=moto=bobo',
            '-e', 'alpha=beta',
            name,
            '/bin/true',
        ])
        service = self.project.get_service(name)
        container = service.containers(stopped=True, one_off=True)[0]
        # env overriden
        self.assertEqual('notbar', container.environment['foo'])
        # keep environement from yaml
        self.assertEqual('world', container.environment['hello'])
        # added option from command line
        self.assertEqual('beta', container.environment['alpha'])
        # make sure a value with a = don't crash out
        self.assertEqual('moto=bobo', container.environment['allo'])

    def test_run_service_without_map_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['run', '-d', 'simple'])
        container = self.project.get_service('simple').containers(one_off=True)[0]

        # get port information
        port_random = container.get_local_port(3000)
        port_assigned = container.get_local_port(3001)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertEqual(port_random, None)
        self.assertEqual(port_assigned, None)

    def test_run_service_with_map_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['run', '-d', '--service-ports', 'simple'])
        container = self.project.get_service('simple').containers(one_off=True)[0]

        # get port information
        port_random = container.get_local_port(3000)
        port_assigned = container.get_local_port(3001)
        port_range = container.get_local_port(3002), container.get_local_port(3003)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertNotEqual(port_random, None)
        self.assertIn("0.0.0.0", port_random)
        self.assertEqual(port_assigned, "0.0.0.0:49152")
        self.assertEqual(port_range[0], "0.0.0.0:49153")
        self.assertEqual(port_range[1], "0.0.0.0:49154")

    def test_run_service_with_explicitly_maped_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['run', '-d', '-p', '30000:3000', '--publish', '30001:3001', 'simple'])
        container = self.project.get_service('simple').containers(one_off=True)[0]

        # get port information
        port_short = container.get_local_port(3000)
        port_full = container.get_local_port(3001)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertEqual(port_short, "0.0.0.0:30000")
        self.assertEqual(port_full, "0.0.0.0:30001")

    def test_run_service_with_explicitly_maped_ip_ports(self):
        # create one off container
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['run', '-d', '-p', '127.0.0.1:30000:3000', '--publish', '127.0.0.1:30001:3001', 'simple'], None)
        container = self.project.get_service('simple').containers(one_off=True)[0]

        # get port information
        port_short = container.get_local_port(3000)
        port_full = container.get_local_port(3001)

        # close all one off containers we just created
        container.stop()

        # check the ports
        self.assertEqual(port_short, "127.0.0.1:30000")
        self.assertEqual(port_full, "127.0.0.1:30001")

    def test_run_with_custom_name(self):
        self.base_dir = 'tests/fixtures/environment-composefile'
        name = 'the-container-name'
        self.dispatch(['run', '--name', name, 'service', '/bin/true'])

        service = self.project.get_service('service')
        container, = service.containers(stopped=True, one_off=True)
        self.assertEqual(container.name, name)

    def test_run_with_networking(self):
        self.require_api_version('1.21')
        client = docker_client(version='1.21')
        self.base_dir = 'tests/fixtures/simple-dockerfile'
        self.dispatch(['--x-networking', 'run', 'simple', 'true'], None)
        service = self.project.get_service('simple')
        container, = service.containers(stopped=True, one_off=True)
        networks = client.networks(names=[self.project.name])
        for n in networks:
            self.addCleanup(client.remove_network, n['Id'])
        self.assertEqual(len(networks), 1)
        self.assertEqual(container.human_readable_command, u'true')

    def test_run_handles_sigint(self):
        proc = start_process(self.base_dir, ['run', '-T', 'simple', 'top'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            running=True))

        os.kill(proc.pid, signal.SIGINT)
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            running=False))

    def test_run_handles_sigterm(self):
        proc = start_process(self.base_dir, ['run', '-T', 'simple', 'top'])
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            running=True))

        os.kill(proc.pid, signal.SIGTERM)
        wait_on_condition(ContainerStateCondition(
            self.project.client,
            'simplecomposefile_simple_run_1',
            running=False))

    def test_rm(self):
        service = self.project.get_service('simple')
        service.create_container()
        kill_service(service)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.dispatch(['rm', '--force'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)
        service = self.project.get_service('simple')
        service.create_container()
        kill_service(service)
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.dispatch(['rm', '-f'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)

    def test_stop(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['stop', '-t', '1'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_pause_unpause(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertFalse(service.containers()[0].is_paused)

        self.dispatch(['pause'], None)
        self.assertTrue(service.containers()[0].is_paused)

        self.dispatch(['unpause'], None)
        self.assertFalse(service.containers()[0].is_paused)

    def test_logs_invalid_service_name(self):
        self.dispatch(['logs', 'madeupname'], returncode=1)

    def test_kill(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['kill'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_kill_signal_sigstop(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['kill', '-s', 'SIGSTOP'], None)

        self.assertEqual(len(service.containers()), 1)
        # The container is still running. It has only been paused
        self.assertTrue(service.containers()[0].is_running)

    def test_kill_stopped_service(self):
        self.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.dispatch(['kill', '-s', 'SIGSTOP'], None)
        self.assertTrue(service.containers()[0].is_running)

        self.dispatch(['kill', '-s', 'SIGKILL'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_restart(self):
        service = self.project.get_service('simple')
        container = service.create_container()
        container.start()
        started_at = container.dictionary['State']['StartedAt']
        self.dispatch(['restart', '-t', '1'], None)
        container.inspect()
        self.assertNotEqual(
            container.dictionary['State']['FinishedAt'],
            '0001-01-01T00:00:00Z',
        )
        self.assertNotEqual(
            container.dictionary['State']['StartedAt'],
            started_at,
        )

    def test_restart_stopped_container(self):
        service = self.project.get_service('simple')
        container = service.create_container()
        container.start()
        container.kill()
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.dispatch(['restart', '-t', '1'], None)
        self.assertEqual(len(service.containers(stopped=False)), 1)

    def test_scale(self):
        project = self.project

        self.dispatch(['scale', 'simple=1'])
        self.assertEqual(len(project.get_service('simple').containers()), 1)

        self.dispatch(['scale', 'simple=3', 'another=2'])
        self.assertEqual(len(project.get_service('simple').containers()), 3)
        self.assertEqual(len(project.get_service('another').containers()), 2)

        self.dispatch(['scale', 'simple=1', 'another=1'])
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.dispatch(['scale', 'simple=1', 'another=1'])
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.dispatch(['scale', 'simple=0', 'another=0'])
        self.assertEqual(len(project.get_service('simple').containers()), 0)
        self.assertEqual(len(project.get_service('another').containers()), 0)

    def test_port(self):
        self.base_dir = 'tests/fixtures/ports-composefile'
        self.dispatch(['up', '-d'], None)
        container = self.project.get_service('simple').get_container()

        def get_port(number):
            result = self.dispatch(['port', 'simple', str(number)])
            return result.stdout.rstrip()

        self.assertEqual(get_port(3000), container.get_local_port(3000))
        self.assertEqual(get_port(3001), "0.0.0.0:49152")
        self.assertEqual(get_port(3002), "0.0.0.0:49153")

    def test_port_with_scale(self):
        self.base_dir = 'tests/fixtures/ports-composefile-scale'
        self.dispatch(['scale', 'simple=2'], None)
        containers = sorted(
            self.project.containers(service_names=['simple']),
            key=attrgetter('name'))

        def get_port(number, index=None):
            if index is None:
                result = self.dispatch(['port', 'simple', str(number)])
            else:
                result = self.dispatch(['port', '--index=' + str(index), 'simple', str(number)])
            return result.stdout.rstrip()

        self.assertEqual(get_port(3000), containers[0].get_local_port(3000))
        self.assertEqual(get_port(3000, index=1), containers[0].get_local_port(3000))
        self.assertEqual(get_port(3000, index=2), containers[1].get_local_port(3000))
        self.assertEqual(get_port(3002), "")

    def test_env_file_relative_to_compose_file(self):
        config_path = os.path.abspath('tests/fixtures/env-file/docker-compose.yml')
        self.dispatch(['-f', config_path, 'up', '-d'], None)
        self._project = get_project(self.base_dir, [config_path])

        containers = self.project.containers(stopped=True)
        self.assertEqual(len(containers), 1)
        self.assertIn("FOO=1", containers[0].get('Config.Env'))

    @mock.patch.dict(os.environ)
    def test_home_and_env_var_in_volume_path(self):
        os.environ['VOLUME_NAME'] = 'my-volume'
        os.environ['HOME'] = '/tmp/home-dir'

        self.base_dir = 'tests/fixtures/volume-path-interpolation'
        self.dispatch(['up', '-d'], None)

        container = self.project.containers(stopped=True)[0]
        actual_host_path = container.get('Volumes')['/container-path']
        components = actual_host_path.split('/')
        assert components[-2:] == ['home-dir', 'my-volume']

    def test_up_with_default_override_file(self):
        self.base_dir = 'tests/fixtures/override-files'
        self.dispatch(['up', '-d'], None)

        containers = self.project.containers()
        self.assertEqual(len(containers), 2)

        web, db = containers
        self.assertEqual(web.human_readable_command, 'top')
        self.assertEqual(db.human_readable_command, 'top')

    def test_up_with_multiple_files(self):
        self.base_dir = 'tests/fixtures/override-files'
        config_paths = [
            'docker-compose.yml',
            'docker-compose.override.yml',
            'extra.yml',

        ]
        self._project = get_project(self.base_dir, config_paths)
        self.dispatch(
            [
                '-f', config_paths[0],
                '-f', config_paths[1],
                '-f', config_paths[2],
                'up', '-d',
            ],
            None)

        containers = self.project.containers()
        self.assertEqual(len(containers), 3)

        web, other, db = containers
        self.assertEqual(web.human_readable_command, 'top')
        self.assertTrue({'db', 'other'} <= set(web.links()))
        self.assertEqual(db.human_readable_command, 'top')
        self.assertEqual(other.human_readable_command, 'top')

    def test_up_with_extends(self):
        self.base_dir = 'tests/fixtures/extends'
        self.dispatch(['up', '-d'], None)

        self.assertEqual(
            set([s.name for s in self.project.services]),
            set(['mydb', 'myweb']),
        )

        # Sort by name so we get [db, web]
        containers = sorted(
            self.project.containers(stopped=True),
            key=lambda c: c.name,
        )

        self.assertEqual(len(containers), 2)
        web = containers[1]

        self.assertEqual(set(web.links()), set(['db', 'mydb_1', 'extends_mydb_1']))

        expected_env = set([
            "FOO=1",
            "BAR=2",
            "BAZ=2",
        ])
        self.assertTrue(expected_env <= set(web.get('Config.Env')))
