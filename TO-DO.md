[ ] Add _ as argument in functions calls

[ ] Think about x == |a, b| -> (x == a || x == b)

use std/files &fs

fs.read("file.txt")
fs.print("file.txt")
fs.write("file.txt", "some text", 1)
fs.writeLine("file.txt", 1, "text")
fs.readLine("file.txt", 1)
fs.mkdir("../", "mydir/")
fs.copy("file.txt", "../mydir/file.txt")
fs.mkdir("", "dir")
fs.move("file.txt", "dir/")
fs.move("../mydir/", "dir/")
fs.isReadable("file.txt") // false, if file is sucured
fs.isWriteable("file.txt") // false, if ReadOnly rights
fs.delete("file.txt")

use skdev/iamtowvee &o

dict windows = { 0: { name: "...", width: 1280, height: 1080, content: {
    figure1: [ // квадрат
        [1.00, 1.00, null, null],
        [1.00, 1.00, null, null],
        [1.00, -1.00, null, null],
        [-1.00, -1.00, null, null]
    ]
} }, ... }

o.init(int id, string name = "Orvex Window", int width = 1280, int height = 720, bool cursor = true, borders = true, fullscreen = false)
o.close(int id)
o.windows.0.width = 1920
o.windows.0.height = 1080
o.refresh(int id)
o.zoom(int id, float scale)
o.move(int id, double x, double y)
o.addBezier(int id, string id, arr[double] points)
o.editBezier(int id, string id, arr[double] newPoints)
o.removeBezier(int id, string id)
