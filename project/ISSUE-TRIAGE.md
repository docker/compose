Triaging of issues
------------------

The docker-compose issue triage process follows
https://github.com/docker/docker/blob/master/project/ISSUE-TRIAGE.md
with the following additions or exceptions.


### Classify the Issue

The following labels are provided in additional to the standard labels:

| Kind         | Description                                                       |
|--------------|-------------------------------------------------------------------|
| kind/cleanup | A refactor or improvement that is related to quality not function |
| kind/parity  | A request for feature parity with docker cli                      |


### Functional areas

Most issues should fit into one of the following functional areas:

| Area            |
|-----------------|
| area/build      |
| area/cli        |
| area/config     |
| area/logs       |
| area/networking |
| area/packaging  |
| area/run        |
| area/scale      |
| area/tests      |
| area/up         |
| area/volumes    |
