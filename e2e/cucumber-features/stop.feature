Feature: Stop

Background:
    Given a compose file
        """
        services:
          should_fail:
            image: alpine
            command: ['sh', '-c', 'exit 123']
          sleep: # will be killed
            image: alpine
            command: ping localhost
            init: true
        """

Scenario: Cascade stop
    When I run "compose up --abort-on-container-exit"
    Then the output contains "should_fail-1 exited with code 123"
    And the output contains "Aborting on container exit..."
    And the exit code is 123

Scenario: Exit code from
    When I run "compose up --exit-code-from should_fail"
    Then the output contains "should_fail-1 exited with code 123"
    And the output contains "Aborting on container exit..."
    And the exit code is 123

# TODO: this is currently not working propagating the exit code properly
#Scenario: Exit code from (cascade stop)
#    When I run "compose up --exit-code-from sleep"
#    Then the output contains "should_fail-1 exited with code 123"
#    And the output contains "Aborting on container exit..."
#    And the exit code is 143

Scenario: Exit code from unknown service
    When I run "compose up --exit-code-from unknown"
    Then the output contains "no such service: unknown"
    And the exit code is 1
