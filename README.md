The Q Language Export Tool
========

Export Go package to Go+ module

The Go+ Language : https://github.com/qiniu/goplus

```
Usage:
  qexport [option] [ std | packages]

The packages for go package list or std for golang all standard packages.

  -filter string
    	optional set export filter regular expression list, separated by spaces.
  -outdir string
    	optional set export output root path (default "./lib")
```   

Example:

	qexport -outdir . std

	qexport -outdir . -filter "Replacer" strings

	qexport -outdir . runtime math regexp

