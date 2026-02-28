---
description: Common bash commands - file operations, text processing, system operations, networking.
metadata:
    nanogrip:
        requires:
            bins:
                - bash
name: bash
---

# Bash Skill

Use bash for common system operations and text processing.

## File Operations

### Listing files
```bash
ls -la                    # list all with details
ls -lh                    # human readable sizes
ls -t                     # sort by time
ls -S                     # sort by size
ls -1                     # one per line
ls -a                     # show hidden
ls -R                     # recursive
```

### File info
```bash
file filename              # file type
stat filename             # detailed info
md5sum filename           # checksum
sha256sum filename        # SHA256
```

### Copy/Move/Remove
```bash
cp source dest
cp -r source/ dest/       # recursive
cp -p preserve            # preserve attributes
mv source dest
rm filename
rm -rf directory/         # force recursive
```

### Create directories
```bash
mkdir dirname
mkdir -p path/to/dir      # create parents
mkdir -m 755 dirname      # set permissions
```

### Touch/Create files
```bash
touch filename
> filename                # truncate to zero
echo "content" > file    # write to file
```

### Read files
```bash
cat filename              # entire file
head -n 20 filename      # first 20 lines
tail -n 20 filename      # last 20 lines
tail -f filename         # follow
less filename            # paginated view
```

## Text Processing

### grep - Search
```bash
grep "pattern" file
grep -r "pattern" dir/   # recursive
grep -i "pattern" file   # case insensitive
grep -n "pattern" file   # line numbers
grep -v "pattern" file   # invert match
grep -E "regex" file     # extended regex
```

### sed - Stream editor
```bash
sed 's/old/new/' file              # replace first
sed 's/old/new/g' file             # replace all
sed -i 's/old/new/g' file         # in-place
sed '/pattern/d' file              # delete lines
sed -n '1,5p' file                # print lines 1-5
```

### awk - Text processing
```bash
awk '{print $1}' file              # print first column
awk -F',' '{print $1}' file       # comma separator
awk 'NR>1' file                   # skip header
awk '{sum+=$1} END {print sum}'  # sum column
```

### sort/uniq
```bash
sort file
sort -u file                      # unique
sort -k2 file                     # sort by column 2
uniq file                         # remove duplicates
uniq -c file                      # count occurrences
```

### cut
```bash
cut -d',' -f1 file               # first field
cut -c1-10 file                  # characters 1-10
```

### wc - Word count
```bash
wc -l file                       # lines
wc -w file                       # words
wc -c file                       # bytes
```

## System Operations

### Processes
```bash
ps aux                           # all processes
ps -ef                          # full format
top                             # interactive
htop                            # better top
pkill process-name
kill process-id
kill -9 process-id              # force kill
```

### Memory/Disk
```bash
free -h                         # memory
df -h                          # disk usage
du -sh directory/              # directory size
du -h --max-depth=1            # breakdown
```

### System info
```bash
uname -a
hostname
uptime
whoami
date
cal
```

### Package management (Debian/Ubuntu)
```bash
apt update
apt upgrade
apt install package
apt remove package
apt search keyword
dpkg -l                        # list installed
```

### Package management (RHEL/CentOS)
```bash
yum update
yum install package
yum remove package
yum search keyword
rpm -qa                        # list installed
```

### Package management (macOS)
```bash
brew update
brew install package
brew uninstall package
brew list
```

## Networking

### Connectivity
```bash
ping -c 4 host
curl -I url                    # headers only
wget url
curl -s url                    # silent
```

### Network tools
```bash
netstat -tulpn                # listening ports
ss -tulpn                     # modern netstat
ip addr                       # IP addresses
ip route                      # routing table
```

### SSH
```bash
ssh user@host
ssh -i keyfile user@host
scp file user@host:/path/
rsync -avz source/ dest:/
```

## Compression

### tar
```bash
tar -cvf archive.tar dir/
tar -czvf archive.tar.gz dir/
tar -xvf archive.tar
tar -xzvf archive.tar.gz
tar -tvf archive.tar          # list contents
```

### zip
```bash
zip -r archive.zip dir/
unzip archive.zip
unzip -l archive.zip          # list contents
```

### gzip
```bash
gzip file
gunzip file.gz
gzip -k file                 # keep original
```

## Permissions

### chmod
```bash
chmod 755 file               # rwxr-xr-x
chmod +x file               # executable
chmod -R 755 dir/           # recursive
```

### chown
```bash
chown user:group file
chown -R user:group dir/
```

## Environment Variables

### Display
```bash
printenv
echo $VAR
env | grep PATTERN
```

### Set
```bash
export VAR=value
VAR=value command            # temporary
```

## Variables and Math

### Variables
```bash
VAR="hello"
echo $VAR
echo ${VAR}
```

### Math
```bash
echo $((1 + 2))
echo $((10 % 3))
VAR=$((VAR + 1))
```

### Arrays
```bash
arr=(one two three)
echo ${arr[0]}
echo ${arr[@]}
```

## Loops and Conditionals

### For loop
```bash
for f in *.txt; do echo "$f"; done
for i in {1..5}; do echo "$i"; done
```

### While loop
```bash
while read line; do echo "$line"; done < file
```

### If statement
```bash
if [ -f file ]; then echo "exists"; fi
if [ -d dir ]; then echo "dir exists"; fi
if [ -z "$VAR" ]; then echo "empty"; fi
```

## Useful One-liners

### Find files
```bash
find . -name "*.log"
find . -type f -mtime -7     # modified 7 days
find . -type d -empty        # empty directories
```

### xargs
```bash
find . -name "*.log" | xargs rm
ls *.txt | xargs -I {} echo {}
```

### Chain commands
```bash
cmd1 && cmd2                 # run if first succeeds
cmd1 || cmd2                 # run if first fails
cmd1 | cmd2                  # pipe
cmd > file 2>&1              # redirect stdout and stderr
```
