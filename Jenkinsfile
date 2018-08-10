#!groovy

def image

def buildImage = { ->
  wrappedNode(label: "ubuntu && !zfs", cleanWorkspace: true) {
    stage("build image") {
      checkout(scm)
      def imageName = "dockerbuildbot/compose:${gitCommit()}"
      image = docker.image(imageName)
      try {
        image.pull()
      } catch (Exception exc) {
        image = docker.build(imageName, ".")
        image.push()
      }
    }
  }
}

def get_versions = { int number ->
  def docker_versions
  wrappedNode(label: "ubuntu && !zfs") {
    def result = sh(script: """docker run --rm \\
        --entrypoint=/code/.tox/py27/bin/python \\
        ${image.id} \\
        /code/script/test/versions.py -n ${number} docker/docker-ce recent
      """, returnStdout: true
    )
    docker_versions = result.split()
  }
  return docker_versions
}

def runTests = { Map settings ->
  def dockerVersions = settings.get("dockerVersions", null)
  def pythonVersions = settings.get("pythonVersions", null)

  if (!pythonVersions) {
    throw new Exception("Need Python versions to test. e.g.: `runTests(pythonVersions: 'py27,py36')`")
  }
  if (!dockerVersions) {
    throw new Exception("Need Docker versions to test. e.g.: `runTests(dockerVersions: 'all')`")
  }

  { ->
    wrappedNode(label: "ubuntu && !zfs", cleanWorkspace: true) {
      stage("test python=${pythonVersions} / docker=${dockerVersions}") {
        checkout(scm)
        def storageDriver = sh(script: 'docker info | awk -F \': \' \'$1 == "Storage Driver" { print $2; exit }\'', returnStdout: true).trim()
        echo "Using local system's storage driver: ${storageDriver}"
        sh """docker run \\
          -t \\
          --rm \\
          --privileged \\
          --volume="\$(pwd)/.git:/code/.git" \\
          --volume="/var/run/docker.sock:/var/run/docker.sock" \\
          -e "TAG=${image.id}" \\
          -e "STORAGE_DRIVER=${storageDriver}" \\
          -e "DOCKER_VERSIONS=${dockerVersions}" \\
          -e "BUILD_NUMBER=\$BUILD_TAG" \\
          -e "PY_TEST_VERSIONS=${pythonVersions}" \\
          --entrypoint="script/test/ci" \\
          ${image.id} \\
          --verbose
        """
      }
    }
  }
}

buildImage()

def testMatrix = [failFast: true]
def docker_versions = get_versions(2)

for (int i = 0; i < docker_versions.length; i++) {
  def dockerVersion = docker_versions[i]
  testMatrix["${dockerVersion}_py27"] = runTests([dockerVersions: dockerVersion, pythonVersions: "py27"])
  testMatrix["${dockerVersion}_py36"] = runTests([dockerVersions: dockerVersion, pythonVersions: "py36"])
  testMatrix["${dockerVersion}_py37"] = runTests([dockerVersions: dockerVersion, pythonVersions: "py37"])
}

parallel(testMatrix)
