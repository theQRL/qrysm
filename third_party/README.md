# Third Party Package Patching

This directory includes local patches to third party dependencies we use in Qrysm. Sometimes,
we need to make a small change to some dependency for ease of use in Qrysm without wanting
to maintain our own fork of the dependency ourselves. Our build tool, [Bazel](https://bazel.build)
allows us to include patches in a seamless manner based on simple diff rules.

This README outlines how patching works in Qrysm and an explanation of previously
created patches. 

**Given maintaining a patch can be difficult and tedious,
patches are NOT the recommended way of modifying dependencies in Qrysm 
unless really needed**

## Table of Contents

- [Prerequisites](#prerequisites)
- [Creating a Patch](#creating-a-patch)
- [Configuring Bazel](#configuring-bazel)

## Prerequisites

**Bazel Installation:**
  - The latest release of [Bazel](https://docs.bazel.build/versions/master/install.html)
  - A modern UNIX operating system (MacOS included)

## Creating a Patch

To create a patch, we need an original version of a dependency which we will refer to as `a`
and the patched version referred to as `b`. 

```
cd /tmp
git clone https://github.com/someteam/somerepo a
git clone https://github.com/someteam/somerepo b && cd b
```
Then, make all your changes in `b` and finally create the diff of all your changes as follows:
```
cd ..
diff -ur --exclude=".git" a b > $GOPATH/src/github.com/theQRL/qrysm/third_party/YOURPATCH.patch
```

## Configuring Bazel

Next, tell Bazel about the patch by updating the dependency definition in `MODULE.bazel`.
Here's an example using a `go_deps.module_override`:

```
go_deps.module_override(
    patches = ["//third_party:com_github_libp2p_go_libp2p_pubsub-gogo.patch"],
    path = "github.com/libp2p/go-libp2p-pubsub",
)
```

Now, when used in Qrysm, the dependency you patched will have the patched modifications
when you run your code.
