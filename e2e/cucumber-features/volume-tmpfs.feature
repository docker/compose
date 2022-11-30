Feature: Volume: tmpfs

Background:
    Given a compose file
        """
        services:
          svc:
            image: busybox
            volumes:
              - type: tmpfs
                target: /volumes/tmpfs
                tmpfs:
                  size: 2M
                  mode: 0o647
        """

Scenario: tmpfs Permissions Set
    When I run "compose run --rm svc stat -c "%a" /volumes/tmpfs"
    Then the output contains "647"

Scenario: tmpfs Size Set
    When I run "compose run --rm svc sh -c 'df /volumes/tmpfs | tail -n1 | awk '"'"'{print $4}'"'"'' "
    Then the output contains "2048"
