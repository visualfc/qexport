The Q Language Export Tool
========

Export go package to qlang module

The Q Language : https://github.com/qiniu/qlang

```
Usage:
  qexport [option] [ std | packages]

The packages for go package list or std for golang all standard packages.

  -filter string
    	optional set export filter regular expression list, separated by spaces.
  -outdir string
    	optional set export output root path (default "./qlang")
```   

Example:

	qexport -outdir . std

	qexport -outdir . -filter "Replacer" strings

	qexport -outdir . runtime math regexp

