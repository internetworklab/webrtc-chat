#import "@preview/cetz:0.3.4"

#set page(width: auto, height: auto, margin: 1cm)

#cetz.canvas({
  import cetz.draw: *

  // Colors
  let agent-color   = rgb(135, 206, 250)   // light blue — both agents
  let server-color  = rgb(144, 238, 144)   // light green
  let arrow-color   = rgb(60, 60, 60)
  let text-color    = rgb(30, 30, 30)

  // --- Agents Group (left) ---
  rect((-6.5, -3.5), (-0.5, 3.5), fill: rgb(245,250,255), stroke: 1pt + rgb(150,150,150), radius: 0.3cm, name: "agents-group")

  // Browser Client (top-left)
  rect((-5.5, 1), (-1.5, 3), fill: agent-color, stroke: 1.5pt + text-color, radius: 0.3cm, name: "browser")
  content(("browser.center"), [
    #set text(font: "DejaVu Sans", size: 12pt, weight: "bold", fill: text-color)
    Browser Client
    #v(2pt)
    #set text(size: 9pt, weight: "regular")
    webrtc-web
  ])

  // Bot Agents (bottom-left)
  rect((-5.5, -3), (-1.5, -1), fill: agent-color, stroke: 1.5pt + text-color, radius: 0.3cm, name: "bots")
  content(("bots.center"), [
    #set text(font: "DejaVu Sans", size: 12pt, weight: "bold", fill: text-color)
    Bot Agents
    #v(2pt)
    #set text(size: 9pt, weight: "regular")
    webrtc-agents
  ])

  // Agents label
  content((-3.5, 3.1), [
    #set text(font: "DejaVu Sans", size: 10pt, weight: "bold", fill: rgb(100,100,100))
    Agents
  ])

  // Signalling Server (right side)
  rect((2, -1), (6, 1), fill: server-color, stroke: 1.5pt + text-color, radius: 0.3cm, name: "server")
  content(("server.center"), [
    #set text(font: "DejaVu Sans", size: 12pt, weight: "bold", fill: text-color)
    Signalling Server
    #v(2pt)
    #set text(size: 9pt, weight: "regular")
    webrtc-server
  ])

  // --- Arrows ---

  // Browser -> Server (WebSocket)
  line((-1.5, 2), (2, 0.5), stroke: 1.5pt + arrow-color, mark: (end: ">", start: "<"))
  content((0.8, 2.0), [
    #set text(font: "DejaVu Sans", size: 9pt, fill: text-color)
    WebSocket
  ])

  // Bot Agents -> Server (WebSocket)
  line((-1.5, -2), (2, -0.5), stroke: 1.5pt + arrow-color, mark: (end: ">", start: "<"))
  content((0.8, -1.0), [
    #set text(font: "DejaVu Sans", size: 9pt, fill: text-color)
    WebSocket
  ])

  // P2P between agents (after SDP exchange)
  line((-3.5, -1), (-3.5, 1), stroke: (dash: "dashed", thickness: 1.5pt, paint: arrow-color), mark: (end: ">", start: "<"))
  content((-5.3, 0), [
    #set text(font: "DejaVu Sans", size: 9pt, fill: text-color)
    P2P WebRTC
  ])

  // --- Note ---
  content((0, -4.2), [
    #set text(font: "DejaVu Sans", size: 9pt, fill: rgb(80,80,80))
    Agents communicate point-to-point once SDP is exchanged via the signalling server.
  ])
})
