from __future__ import absolute_import
from __future__ import unicode_literals
import os
import inspect

class Plugin(object):
    def __init__(self):
        self.path = os.path.abspath(inspect.getfile(self.__class__))
        print(self.path)
        self.name = os.path.dirname(self.path)
        self.description = ''

    def install(self):
        return True

    def uninstall(self):
        return True

    def configure(self):
        return True
