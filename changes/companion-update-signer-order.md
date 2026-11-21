### General

- The companion update checks that the downloaded app is signed with the project certificate before it runs the app's CLI or companion to read their versions. Before, a replaced disk image and `checksums.txt` could run code when `gh` was missing or logged out, before the certificate check refused the app.
