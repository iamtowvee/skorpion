[V] Add _ as argument in functions calls

[X] Think about x == |a, b| -> (x == a || x == b)

[V] Refactor realisation Dictionary to HashMap

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

use skdev/orvex &o

// Ед. изм.: rem, em, px, %, vw, vh
const defStyles = {
    "global:__orvex__root-default-class__": {
        color: "#000000", // string
        display: "block", // string
        width: "100%", // string
        height: "100%", // string
        paddingLeft: "0px", // string
        paddingTop: "0px", // string
        paddingRight: "0px", // string
        paddingBottom: "0px", // string
        marginLeft: "10px", // string
        marginTop: "10px", // string
        marginRight: "10px", // string
        marginBottom: "10px", // string
        backgroundColor: "#ffffff", // string
        backgroundImage: null, // string
        opacity: 1f, // float
        fontFamily: "'Arial'", // string
        textWeight: 300, // int
        textSize: "16px", // string
        wordSpaceSize: "0.5rem", // string
        letterSpaceSize: "0.01rem", // string
        layer: -1, // int
        borderWidth: "0px", // string
        borderColor: "#000000", // string
        shadow: "0px 0px 0px", // string
        shadowColor: "#000000", // string
        textShadow: "0px 0px 0px", // string
        textShadowColor: "#000000", // string
        position: , // string
        top: null, // string
        right: null, // string
        bottom: null, // string
        left: null, // string
        gap: "5px", // string
        transition: null, // arr
        transform: null, // dict
        filters: null // arr
    }
}

dict windows = { 0: {
    name: "Orvex Window", // string
    icon: ["./logo.ico", "./logo.png"], // arr
    mode: "windowed", // string
    resizable: false, // bool
    topOnly: false, // bool
    frame: true, // bool
    posx: -1, // int
    posy: -1, // int
    center: true, // bool
    minimizable: true, // bool
    show: true, // bool
    showTaskbar: true, // bool
    maximizable: true, // bool
    isDevToolsEnabled: true, // bool
    movable: true, // bool
    width: 1280, // int
    height: 720, // int
    contextIsolation: true, // bool
    defines: {
        fgColor: "#efefea",
        bgColor: "#3d3d3a",
        accent: "#aef4fe"
    },
    stylesheet: {
        "global": {
            backgroundColor: "$bgColor"
        },
        "header": {
            color: "$accent"
        },
        "text": {
            color: "$fgColor"
        }
    },
    content: [
        {
            type: "header",
            params: null,
            include: [
                {
                    type: "raw",
                    params: null,
                    include: "Title"
                }
            ]
        },
        {
            type: "text",
            params: null,
            include: [
                {
                    type: "raw",
                    params: null,
                    include: "This line is very cool!"
                }
            ]
        },
        {
            type: "table",
            params: null,
            include: [
                {
                    type: "standline",
                    params: {
                        column: 0,
                        line: null,
                        id: null,
                        class: "__orvex__root-default-class__",
                        hoverEvent: null,
                        clickEvent: null,
                        tip: null,
                        activateKey: null,
                        custom: null
                    },
                    include: [
                        {
                            type: "line",
                            params: {
                                line: 0,
                                column: null,
                                id: null,
                                class: "__orvex__root-default-class__",
                                hoverEvent: null,
                                clickEvent: null,
                                tip: null,
                                activateKey: null,
                                custom: null
                            },
                            include: [
                                {
                                    type: "raw",
                                    params: null,
                                    include: ""
                                }
                            ]
                        }
                    ]
                }
            ]
        }
    ]
}, ... }

// type, params, include

o.init(int id, string name = "Orvex Window", int width = 1280, int height = 720, bool cursor = true, borders = true, fullscreen = false)
o.close(int id)
o.windows.0.width = 1920
o.windows.0.height = 1080
o.refresh(int id)
o.load(int id, string path)
o.push(int id, dict content)

o.find(int id, string uid, dict cp)

file.orv example:
```
$mutColor "#7b7b7b"

<text|header>:mut {color "$mutColor"}

[header ()("Title")]
[text ()("Text inline + text inline = ")]
[text (class "mut")("One line of text")]
```
