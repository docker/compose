#!groovy

def buildImage = { String baseImage ->
  def imageName = "compose:${baseImage}"
  wrappedNode(label: "ubuntu && amd64 && !zfs", cleanWorkspace: true) {
    stage("build image for \"${baseImage}\"") {
      checkout(scm)
      sh """docker build -t ${imageName} \\
            --target build \\
            --build-arg BUILD_PLATFORM="${baseImage}" \\
            .\\
      """
      sh "docker save -o baseImage.tar ${imageName}"
      stash( includes:"baseImage.tar", name: baseImage )
      echo "${imageName}"
    }
  }
  return imageName
}

def get_versions = { String baseImage, int number ->
  def docker_versions
  wrappedNode(label: "ubuntu && amd64 && !zfs") {
    unstash baseImage
    sh "docker load -i baseImage.tar"
    def result = sh(script: """docker run --rm \\
        --entrypoint=/code/.tox/py27/bin/python \\
        compose:${baseImage} \\
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
  def baseImage = settings.get("baseImage", null)
  def imageName = settings.get("image", null)

  if (!pythonVersions) {
    throw new Exception("Need Python versions to test. e.g.: `runTests(pythonVersions: 'py27,py37')`")
  }
  if (!dockerVersions) {
    throw new Exception("Need Docker versions to test. e.g.: `runTests(dockerVersions: 'all')`")
  }

  { ->
    wrappedNode(label: "ubuntu && amd64 && !zfs", cleanWorkspace: true) {
      stage("test python=${pythonVersions} / docker=${dockerVersions} / baseImage=${baseImage}") {
        unstash baseImage
        sh "docker load -i baseImage.tar"

        checkout(scm)
        def storageDriver = sh(script: 'docker info | awk -F \': \' \'$1 == "Storage Driver" { print $2; exit }\'', returnStdout: true).trim()
        echo "Using local system's storage driver: ${storageDriver}"
        sh """docker run \\
          -t \\
          --rm \\
          --privileged \\
          --volume="\$(pwd)/.git:/code/.git" \\
          --volume="/var/run/docker.sock:/var/run/docker.sock" \\
          -e "TAG=compose:${baseImage}" \\
          -e "STORAGE_DRIVER=${storageDriver}" \\
          -e "DOCKER_VERSIONS=${dockerVersions}" \\
          -e "BUILD_NUMBER=\$BUILD_TAG" \\
          -e "PY_TEST_VERSIONS=${pythonVersions}" \\
          --entrypoint="script/test/ci" \\
          compose:${baseImage} \\
          --verbose
        """
      }
    }
  }
}

def testMatrix = [failFast: true]
def baseImages = ['alpine', 'debian']
def pythonVersions = ['py27', 'py37']
baseImages.each { baseImage ->
  buildImage(baseImage)
  get_versions(baseImage, 2).each { dockerVersion ->
    pythonVersions.each { pyVersion ->
      testMatrix["${baseImage}_${dockerVersion}_${pyVersion}"] = runTests([baseImage: baseImage, dockerVersions: dockerVersion, pythonVersions: pyVersion])
    }
  }
}

parallel(testMatrix)
