# NAME
   runc delete - delete any resources held by the container often used with detached containers

# SYNOPSIS
   runc delete <container-id>

Where "<container-id>" is the name for the instance of the container.

# EXAMPLE
For example, if the container id is "ubuntu01" and runc list currently shows the
status of "ubuntu01" as "destroyed" the following will delete resources held for
"ubuntu01" removing "ubuntu01" from the runc list of containers:  

       # runc delete ubuntu01
