# Project Tooling Diagrams

Generated from [examples/project_tooling.convspec](../../examples/project_tooling.convspec) and [examples/project_tooling.proto](../../examples/project_tooling.proto).

The deterministic HTML report is also checked in at [project_tooling.html](project_tooling.html), but GitHub's repository viewer shows HTML files as source. This Markdown page is the GitHub-rendered view.

## State Machine

<img src="project_tooling_assets/llm_design_workbench_v1_state.png" alt="Project tooling state machine" width="900">

## Actor Protocol Projections

These projections show the send/receive callback surface for each actor in the workbench.

<img src="project_tooling_assets/llm_design_workbench_v1_actor_user.png" alt="User actor protocol projection" width="900">

<img src="project_tooling_assets/llm_design_workbench_v1_actor_browser.png" alt="Browser actor protocol projection" width="900">

<img src="project_tooling_assets/llm_design_workbench_v1_actor_specweb.png" alt="Specweb actor protocol projection" width="900">

<img src="project_tooling_assets/llm_design_workbench_v1_actor_compiler.png" alt="Compiler actor protocol projection" width="900">

<img src="project_tooling_assets/llm_design_workbench_v1_actor_graphviz.png" alt="Graphviz actor protocol projection" width="900">

<img src="project_tooling_assets/llm_design_workbench_v1_actor_openai.png" alt="OpenAI actor protocol projection" width="900">

<img src="project_tooling_assets/llm_design_workbench_v1_actor_repository.png" alt="Repository actor protocol projection" width="900">

## Interaction Scenarios

### Path 1: LLM Text Answer Saved

<img src="project_tooling_assets/llm_design_workbench_v1_path_01.svg" alt="Project tooling LLM text answer saved interaction" width="900">

### Path 2: LLM Patch Proposal Saved

<img src="project_tooling_assets/llm_design_workbench_v1_path_02.svg" alt="Project tooling LLM patch proposal saved interaction" width="900">

### Path 3: Local Compiler Fallback Saved

<img src="project_tooling_assets/llm_design_workbench_v1_path_03.svg" alt="Project tooling local fallback saved interaction" width="900">

### Path 4: Compile Error

<img src="project_tooling_assets/llm_design_workbench_v1_path_04.svg" alt="Project tooling compile error interaction" width="900">
