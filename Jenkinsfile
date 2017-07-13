#!groovy

def image

def buildImage = { ->
  wrappedNode(label: "ubuntu && !zfs", cleanWorkspace: true) {
    stage("build image") {
      echo("Nothing to see here")
    }
  }
}


buildImage()
