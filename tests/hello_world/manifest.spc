name["MyProject"]
version["1.0.0"]
main["./main.sk"]
authors[("iamtowvee")]

channel["stable"]
buildId["42"]

buildOutFile["{name}-{channel}-{version}.exe"]
#! → MyProject-stable-1.0.0.exe

tags[(&channel, "release")]
#! → ["stable", "release"]