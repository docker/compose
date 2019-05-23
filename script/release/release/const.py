from __future__ import absolute_import
from __future__ import unicode_literals

import os


REPO_ROOT = os.path.join(os.path.dirname(__file__), '..', '..', '..')
NAME = 'docker/compose'
COMPOSE_TESTS_IMAGE_BASE_NAME = NAME + '-tests'
BINTRAY_ORG = 'docker-compose'
