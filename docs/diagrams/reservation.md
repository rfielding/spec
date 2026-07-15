# Reservation Protocol Diagrams

Generated from [examples/reservation.convspec](../../examples/reservation.convspec) and [examples/reservation.proto](../../examples/reservation.proto).

The deterministic HTML report is also checked in at [reservation.html](reservation.html), but GitHub's repository viewer shows HTML files as source. This Markdown page is the GitHub-rendered view to link from issues, pull requests, and project notes.

## State Machine

<img src="reservation_assets/reservation_v2_state.png" alt="Reservation v2 state machine" width="900">

## Actor Protocol Projections

These projections show the send/receive callback surface for each actor. The interaction scenarios below remain the primary implementation view.

<img src="reservation_assets/reservation_v2_actor_client.png" alt="Reservation client actor protocol projection" width="900">

<img src="reservation_assets/reservation_v2_actor_broker.png" alt="Reservation broker actor protocol projection" width="900">

<img src="reservation_assets/reservation_v2_actor_supplier.png" alt="Reservation supplier actor protocol projection" width="900">

## Interaction Scenarios

### Path 1: Confirmed

<img src="reservation_assets/reservation_v2_path_01.svg" alt="Reservation v2 confirmed interaction" width="900">

### Path 2: Cancelled

<img src="reservation_assets/reservation_v2_path_02.svg" alt="Reservation v2 cancelled interaction path 2" width="900">

### Path 3: Cancelled

<img src="reservation_assets/reservation_v2_path_03.svg" alt="Reservation v2 cancelled interaction path 3" width="900">

### Path 4: Expired

<img src="reservation_assets/reservation_v2_path_04.svg" alt="Reservation v2 expired interaction" width="900">

### Path 5: Rejected

<img src="reservation_assets/reservation_v2_path_05.svg" alt="Reservation v2 rejected interaction" width="900">

### Path 6: Cancelled

<img src="reservation_assets/reservation_v2_path_06.svg" alt="Reservation v2 cancelled interaction path 6" width="900">
