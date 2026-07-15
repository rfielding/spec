# Bakery Day Diagrams

Generated from [examples/bakery_day.convspec](../../examples/bakery_day.convspec) and [examples/bakery_day.proto](../../examples/bakery_day.proto).

This stress-test example models a bread bakery day: early bakers, inventory draw, dough mixing, oven carousel turns, cooling, wrapping, truck delivery, storefront sales, Stripe/cash closeout, leftover sorting, charity pickup, waste, and payroll accrual.

The protobuf messages include product-mix and traffic-log fields so later deterministic renderers can draw charts for challah/sourdough/cinnamon mix, loaves sold/donated/wasted, money flow through card/cash sales, payroll, waste loss, charity rebate estimates, and queue load observations.

The deterministic HTML report is also checked in at [bakery_day.html](bakery_day.html), but GitHub's repository viewer shows HTML files as source. This Markdown page is the GitHub-rendered view.

## State Machine

<img src="bakery_day_assets/daily_loaf_flow_v1_state.png" alt="Daily loaf flow state machine" width="900">

## Interaction Scenarios

### Path 1: Sold Out

<img src="bakery_day_assets/daily_loaf_flow_v1_path_01.svg" alt="Bakery day sold out interaction" width="900">

### Path 2: Normal Sales With Charity Pickup

<img src="bakery_day_assets/daily_loaf_flow_v1_path_02.svg" alt="Bakery day charity interaction path 2" width="900">

### Path 3: Normal Sales With Waste

<img src="bakery_day_assets/daily_loaf_flow_v1_path_03.svg" alt="Bakery day waste interaction path 3" width="900">

### Path 4: Slow Sales With Charity Pickup

<img src="bakery_day_assets/daily_loaf_flow_v1_path_04.svg" alt="Bakery day charity interaction path 4" width="900">

### Path 5: Slow Sales With Waste

<img src="bakery_day_assets/daily_loaf_flow_v1_path_05.svg" alt="Bakery day waste interaction path 5" width="900">

### Path 6: Truck Capacity Shortage

<img src="bakery_day_assets/daily_loaf_flow_v1_path_06.svg" alt="Bakery day truck shortage interaction" width="900">

### Path 7: Staff Shortage

<img src="bakery_day_assets/daily_loaf_flow_v1_path_07.svg" alt="Bakery day staff shortage interaction" width="900">

### Path 8: Ingredient Shortage

<img src="bakery_day_assets/daily_loaf_flow_v1_path_08.svg" alt="Bakery day ingredient shortage interaction" width="900">
