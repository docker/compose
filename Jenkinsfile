#!groovy

def buildImage = { String baseImage ->
  def image
  wrappedNode(label: "ubuntu && amd64 && !zfs", cleanWorkspace: true) {
    stage("build image for \"${baseImage}\"") {
      checkout(scm)
      def imageName = "dockerbuildbot/compose:${baseImage}-${gitCommit()}"
      image = docker.image(imageName)
      try {
        image.pull()
      } catch (Exception exc) {
        sh """GIT_COMMIT=\$(script/build/write-git-sha) && \\
            docker build -t ${imageName} \\
            --target build \\
            --build-arg BUILD_PLATFORM="${baseImage}" \\
            --build-arg GIT_COMMIT="${GIT_COMMIT}" \\
            .\\
        """
        sh "docker push ${imageName}"
        echo "${imageName}"
        return imageName
      }
    }
  }
  echo "image.id: ${image.id}"
  return image.id
}

def get_versions = { String imageId, int number ->
  def docker_versions
  wrappedNode(label: "ubuntu && amd64 && !zfs") {
    def result = sh(script: """docker run --rm \\
        --entrypoint=/code/.tox/py27/bin/python \\
        ${imageId} \\
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
        checkout(scm)
        def storageDriver = sh(script: 'docker info | awk -F \': \' \'$1 == "Storage Driver" { print $2; exit }\'', returnStdout: true).trim()
        echo "Using local system's storage driver: ${storageDriver}"
        sh """docker run \\
          -t \\
          --rm \\
          --privileged \\
          --volume="\$(pwd)/.git:/code/.git" \\
          --volume="/var/run/docker.sock:/var/run/docker.sock" \\
          -e "TAG=${imageName}" \\
          -e "STORAGE_DRIVER=${storageDriver}" \\
          -e "DOCKER_VERSIONS=${dockerVersions}" \\
          -e "BUILD_NUMBER=\$BUILD_TAG" \\
          -e "PY_TEST_VERSIONS=${pythonVersions}" \\
          --entrypoint="script/test/ci" \\
          ${imageName} \\
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
  def imageName = buildImage(baseImage)
  get_versions(imageName, 2).each { dockerVersion ->
    pythonVersions.each { pyVersion ->
      testMatrix["${baseImage}_${dockerVersion}_${pyVersion}"] = runTests([baseImage: baseImage, image: imageName, dockerVersions: dockerVersion, pythonVersions: pyVersion])
    }
  }
}

parallel(testMatrix)
