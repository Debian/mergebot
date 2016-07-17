# Mergebot

## Vision

Minimize the number of steps required to accept contributions for Debian packages you maintain.

## Usage instructions

To merge the most recent patch in Debian bug #831331 and build the resulting
package, use:
```
mergebot -source_package=wit -bug=831331
```

Afterwards, introspect the resulting Debian package and git repository.
If both look good, push and upload using the following commands which are
suggested by the `mergebot` invocation above:
```
cd /tmp/mergebot-19384221
(cd repo && git push)
(cd export && debsign *.changes && dput *.changes)
```

See “Future ideas” for how to further streamline this process.

## Dependencies

* `git`
* `sbuild`
* `gbp`
* `devscripts` (pulled in by `gbp` as well)

## Assumptions

* your repository can be cloned using `gbp clone --pristine-tar`
* your repository uses `git` as SCM
* your repository can be built using `gbp buildpackage` with `sbuild`

## Future ideas

Please get in touch in case you’re interested in using or helping with any of
the following features:

* Run `mergebot` automatically for every incoming patch, respond to the bug
  with a report about whether the patch can be merged successfully and whether
  the resulting package builds successfully.
* Add a UI to `mergebot` (web service? email? user script for the BTS?),
  allowing you to have `mergebot` merge, build, push and upload contributions
  on your behalf.
