# Notices

Notted copies and modifies code from quant (github.com/brienze1/quant), under the MIT License
below. Each copied file starts with a header naming its quant source, and is listed in the table.
Notted never imports quant's packages.

## quant license

MIT License

Copyright (c) 2026 Luis Brienze

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Copied files

| Notted file | quant source | Changes |
|---|---|---|
| `internal/infra/paths/paths.go` | `internal/infra/paths/paths.go` (lines 1-28) | renamed to NOTTED_HOME and ~/.notted; added DBPath, AudioDir, ModelsDir |
| `Makefile` | `Makefile` (lines 32-33, 87-88, 107-108) | kept dev/build/test only; added check and frontend gates; no release, brew or network targets |
| `internal/infra/store/store.go` | `internal/infra/db/sqlite.go` (lines 18-44) | path parameter, 0700 folder, busy timeout, one connection, versioned migrations |
