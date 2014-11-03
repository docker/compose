from docker import Client
from docker import tls
import ssl
import os

_created_clients = []

class ClientMaker:

    def __init__(self, verbose=False):
        self.verbose = verbose


    def _get_docker_host(self, service_dict):

        base_url = os.environ.get('DOCKER_HOST')

        if 'docker_host' in service_dict:
            base_url = service_dict['docker_host']

        return base_url


    def _get_docker_cert_path(self, service_dict):

        cert_path = os.environ.get('DOCKER_CERT_PATH', '')

        if 'docker_cert_path' in service_dict:
            cert_path = service_dict['docker_cert_path']

        if cert_path == '':
            cert_path = os.path.join(os.environ.get('HOME', ''), '.docker')

        return cert_path


    def _get_docker_tls_verify(self, service_dict):

        docker_tls_verify = ( os.environ.get('DOCKER_TLS_VERIFY', '') != '' )

        if 'docker_tls_verify' in service_dict:
            docker_tls_verify = ( service_dict['docker_tls_verify'] == 1 )

        return docker_tls_verify


    def get_client(self, service_dict={}):

        base_url = self._get_docker_host(service_dict)
        cert_path = self._get_docker_cert_path(service_dict)
        tls_verify = self._get_docker_tls_verify(service_dict)

        # if the client with the same base_url is created, return it
        for _client in _created_clients:
            if base_url.split('://', 1)[1] == _client.base_url.split('://', 1)[1]:
                return _client

        # Get Tls config
        tls_config = None
        if tls_verify:
            parts = base_url.split('://', 1)
            base_url = '%s://%s' % ('https', parts[1])

            client_cert = (os.path.join(cert_path, 'cert.pem'), os.path.join(cert_path, 'key.pem'))
            ca_cert = os.path.join(cert_path, 'ca.pem')

            tls_config = tls.TLSConfig(
                ssl_version=ssl.PROTOCOL_TLSv1,
                verify=True,
                assert_hostname=False,
                client_cert=client_cert,
                ca_cert=ca_cert,
            )

        # Get the Docker Client
        client = Client(base_url=base_url, tls=tls_config)
        _created_clients.append(client)

        if self.verbose:
            version_info = six.iteritems(client.version())
            log.info("Fig version %s", __version__)
            log.info("Docker base_url: %s", client.base_url)
            log.info("Docker version: %s",
                     ", ".join("%s=%s" % item for item in version_info))
            return verbose_proxy.VerboseProxy('docker', client)

        return client




def docker_client_maker(verbose=False):
    """
    Returns a docker-py client generator which generate docker-py client.
    docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """

    return ClientMaker(verbose)

