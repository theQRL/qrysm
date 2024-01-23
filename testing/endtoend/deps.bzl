load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")  # gazelle:keep

#def e2e_deps():
#    http_archive(
#        name = "web3signer",
#        urls = ["https://artifacts.consensys.net/public/web3signer/raw/names/web3signer.tar.gz/versions/23.3.1/web3signer-23.3.1.tar.gz"],
#        sha256 = "32dfbfd8d5900f19aa426d3519724dd696e6529b7ec2f99e0cb1690dae52b3d6",
#        build_file = "@qrysm//testing/endtoend:web3signer.BUILD",
#        strip_prefix = "web3signer-23.3.1",
#    )
