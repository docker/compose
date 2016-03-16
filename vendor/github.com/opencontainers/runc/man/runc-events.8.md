# NAME
   runc events - display container events such as OOM notifications, cpu, memory, IO and network stats

# SYNOPSIS
   runc events [command options] <container-id>

Where "<container-id>" is the name for the instance of the container.

# DESCRIPTION
   The events command displays information about the container. By default the
information is displayed once every 5 seconds.

# OPTIONS
   --interval "5s"      set the stats collection interval
   --stats              display the container's stats then exit
   
