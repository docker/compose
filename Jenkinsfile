#!groovy

def image

def checkDocs = { ->
  wrappedNode(label: 'linux') {
    deleteDir(); checkout(scm)
    documentationChecker("docs")
  }
}

def buildImage = { ->
  wrappedNode(label: "linux && !zfs") {
    stage("build image") {
      deleteDir(); checkout(scm)
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

def runTests = { Map settings ->
  def dockerVersions = settings.get("dockerVersions", null)
  def pythonVersions = settings.get("pythonVersions", null)

  if (!pythonVersions) {
    throw new Exception("Need Python versions to test. e.g.: `runTests(pythonVersions: 'py27,py34')`")
  }
  if (!dockerVersions) {
    throw new Exception("Need Docker versions to test. e.g.: `runTests(dockerVersions: 'all')`")
  }

  { ->
    wrappedNode(label: "linux && !zfs") {
      stage("test python=${pythonVersions} / docker=${dockerVersions}") {
        deleteDir(); checkout(scm)
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
          --entrypoint="script/ci" \\
          ${image.id} \\
          --verbose
        """
      }
    }
  }
}

def buildAndTest = { ->
  buildImage()
  // TODO: break this out into meaningful "DOCKER_VERSIONS" values instead of all
  parallel(
    failFast: true,
    all_py27: runTests(pythonVersions: "py27", dockerVersions: "all"),
    all_py34: runTests(pythonVersions: "py34", dockerVersions: "all"),
  )
}


parallel(
  failFast: false,
  docs: checkDocs,
  test: buildAndTest
)
