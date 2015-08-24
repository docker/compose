#!/bin/bash -e

# Populate an array with just docker dirs and one with content dirs
docker_dir=(`ls -d /docs/content/docker/*`)
content_dir=(`ls -d /docs/content/*`)

# Loop content not of docker/
#
# Sed to process GitHub Markdown
# 1-2 Remove comment code from metadata block
# 3 Remove .md extension from link text
# 4 Change ](/ to ](/project/ in links
# 5 Change ](word) to ](/project/word)
# 6 Change ](../../ to ](/project/
# 7 Change ](../ to ](/project/word)
#
for i in "${content_dir[@]}"
do
   :
   case $i in
      "/docs/content/windows")
      ;;
      "/docs/content/mac")
      ;;
      "/docs/content/linux")
      ;;
      "/docs/content/docker")
         y=${i##*/}
         find $i -type f -name "*.md" -exec sed -i.old \
         -e '/^<!.*metadata]>/g' \
         -e '/^<!.*end-metadata.*>/g' {} \;
      ;;
      *)
        y=${i##*/}
        find $i -type f -name "*.md" -exec sed -i.old \
        -e '/^<!.*metadata]>/g' \
        -e '/^<!.*end-metadata.*>/g' \
        -e 's/\(\]\)\([(]\)\(\/\)/\1\2\/'$y'\//g' \
        -e 's/\(\][(]\)\([A-z].*\)\(\.md\)/\1\/'$y'\/\2/g' \
        -e 's/\([(]\)\(.*\)\(\.md\)/\1\2/g'  \
        -e 's/\(\][(]\)\(\.\/\)/\1\/'$y'\//g' \
        -e 's/\(\][(]\)\(\.\.\/\.\.\/\)/\1\/'$y'\//g' \
        -e 's/\(\][(]\)\(\.\.\/\)/\1\/'$y'\//g' {} \;
      ;;
      esac
done

#
#  Move docker directories to content
#
for i in "${docker_dir[@]}"
do
   :
    if [ -d $i ]
      then
        mv $i /docs/content/
      fi
done

rm -rf /docs/content/docker
