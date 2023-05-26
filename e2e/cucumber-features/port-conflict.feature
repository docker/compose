Feature: Report port conflicts

Background:
    Given a compose file
        """
        services:
          web:
            image: nginx
            ports:
            - 31415:80
        """
    And I run "docker rm -f nginx-pi-31415"

Scenario: Reports a port allocation conflict with another container
    Given I run "docker run -d -p 31415:80 --name nginx-pi-31415 nginx"
    When I run "compose up -d"
    Then the output contains "port is already allocated"
    And the exit code is 1

Scenario: Reports a port conflict with some other process
    Given a process listening on port 31415
    When I run "compose up -d"
    Then the output contains "address already in use"
    And the exit code is 1

Scenario: Cleanup
    Given I run "docker rm -f nginx-pi-31415"

