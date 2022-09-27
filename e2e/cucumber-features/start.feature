Feature: Start

Background:
    Given a compose file
        """
        services:
          simple:
            image: alpine
            command: top
          another:
            image: alpine
            command: top
        """

Scenario: Start single service
    When I run "compose create"
    Then the output contains "simple-1  Created"
    And the output contains "another-1  Created"
    Then I run "compose start another"
    And service "another" is "running"
    And service "simple" is "created"
