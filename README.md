# goscp - scp client in Go

Limitations
* Only password authentication.
* Only upload and download of files, no handling of directories
* No checking of destination key!

Features
* Ability to limit bandwidth (Only when destination is remote).

Why?
* I had the requirement to send files over scp on low bandwidth infrastructure
shared with critical equipment. I needed a way to make sure my transfers don't 
wipe out everything else during file transfers

Example
```sh
goscp -limit 20480 file user@host:file
```
