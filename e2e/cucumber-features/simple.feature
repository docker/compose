Feature: Simple service up

Background:
    Given a compose file
        """
        services:
          simple:
            image: alpine
            command: top
        """

Scenario: compose up
    When I run "compose up -d"
    Then the output contains "simple-1  Started"
    And service "simple" is "running"
