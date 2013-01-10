# goscp - scp client in Go

Limitations
* All source files must be local (aka. this is upload only)
* Only password authentication.

Features
* Ability to limit bandwidth.

Why?
* I had the requirement to send files over scp on low bandwidth infrastructure
shared with critical equipment. I needed a way to make sure my transfers don't 
wipe out everything else during file transfers

Example
```sh
goscp -limit 20480 file user@host:file
```

[![Build Status](https://secure.travis-ci.org/howeyc/goscp.png?branch=master)](http://travis-ci.org/howeyc/goscp)
