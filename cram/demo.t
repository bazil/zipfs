  $ umask 022
  $ mkdir -p data/buried/deep
  $ echo hello, world >data/greeting
  $ echo gold >data/buried/deep/loot
  $ find data | xargs touch -t 201312111203 --
  $ unset TIME_STYLE
  $ ( cd data && zip -r -q ../archive.zip . )
  $ wait_for_mount() { while [ "$(stat -f --printf='%T' mnt)" != "fuseblk" ]; do sleep 0.1; done }

Mount it:

  $ mkdir mnt
  $ zipfs archive.zip mnt &

  $ wait_for_mount

Lookup directory entries:

  $ ls -ld mnt/greeting
  -rw-r--r-- 1 root root 13 Dec 11  2013 mnt/greeting
  $ ls -ld mnt/buried
  drwxr-xr-x 1 root root 0 Dec 11  2013 mnt/buried

Read file contents:

  $ cat mnt/greeting
  hello, world
  $ cat mnt/buried/deep/loot
  gold

Readdir (the "total 0" is not correct, but that doesn't matter):

  $ ls -l mnt
  total 0
  drwxr-xr-x 1 root root  0 Dec 11  2013 buried
  -rw-r--r-- 1 root root 13 Dec 11  2013 greeting
  $ ls -l mnt/buried
  total 0
  drwxr-xr-x 1 root root 0 Dec 11  2013 deep


Unmount (for OS X, use `umount mnt`):

  $ while ! fusermount -u mnt; do sleep 0.1; done
  $ wait
