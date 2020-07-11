from distutils.version import LooseVersion


class ComposeVersion(LooseVersion):
    """ A hashable version object """
    def __hash__(self):
        return hash(self.vstring)
