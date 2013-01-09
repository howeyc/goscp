# goscp - scp client in Go

* Only one source and one dest.
* Only password authentication.
* Ability to limit bandwidth.

Example
```sh
goscp -limit 20480 file user@host:file
```

[![Build Status](https://secure.travis-ci.org/howeyc/goscp.png?branch=master)](http://travis-ci.org/howeyc/goscp)
