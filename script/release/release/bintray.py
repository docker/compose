from __future__ import absolute_import
from __future__ import unicode_literals

import json
import os

import requests

from .const import NAME


class BintrayAPI(requests.Session):
    def __init__(self, api_key, user, *args, **kwargs):
        super(BintrayAPI, self).__init__(*args, **kwargs)
        self.auth = (user, api_key)
        self.base_url = 'https://api.bintray.com/'

    def create_repository(self, subject, repo_name, repo_type='generic'):
        url = '{base}repos/{subject}/{repo_name}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name,
        )
        data = {
            'name': repo_name,
            'type': repo_type,
            'private': False,
            'desc': 'Automated release for {}: {}'.format(NAME, repo_name),
            'labels': ['docker-compose', 'docker', 'release-bot'],
        }
        return self.post_json(url, data)

    def repository_exists(self, subject, repo_name):
        url = '{base}/repos/{subject}/{repo_name}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name,
        )
        result = self.get(url)
        if result.status_code == 404:
            return False
        result.raise_for_status()
        return True

    def delete_repository(self, subject, repo_name):
        url = '{base}repos/{subject}/{repo_name}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name,
        )
        return self.delete(url)

    def create_package(self, subject, repo_name, package_name):
        url = '{base}packages/{subject}/{repo_name}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name
        )
        data = {
            'name': package_name,
            'desc': 'auto',
            'website_url': 'https://docs.docker.com/compose/',
            'licenses': ['Apache-2.0'],
            'vcs_url': 'git@github.com:docker/compose.git',
        }
        return self.post_json(url, data)

    def create_version(self, subject, repo_name, package_name):
        url = '{base}packages/{subject}/{repo_name}/{package_name}/versions'.format(
            base=self.base_url, subject=subject, repo_name=repo_name, package_name=package_name
        )
        data = {
            'name': repo_name,
            'desc': 'Automated build of the {repo_name} branch'.format(repo_name=repo_name),
        }
        return self.post_json(url, data)

    def upload_file(self, subject, repo_name, package_name, file):
        url = '{base}content/{subject}/{repo_name}/{filename}'.format(
            base=self.base_url, subject=subject, repo_name=repo_name, filename=os.path.basename(file)
        )
        headers = {
            'X-Bintray-Package': package_name,
            'X-Bintray-Version': repo_name,
            'X-Bintray-Override': 1,
            'X-Bintray-Publish': 1,
        }
        data = open(file, 'rb').read()
        return self.put(url, data=data, headers=headers)

    def post_json(self, url, data, **kwargs):
        if 'headers' not in kwargs:
            kwargs['headers'] = {}
        kwargs['headers']['Content-Type'] = 'application/json'
        return self.post(url, data=json.dumps(data), **kwargs)
