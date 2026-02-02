Use .venv 
Use python only for vizualization purposes. All data processing is inside of Go because it is data intensive

u use tmux skill to run some work and think on it's result at the same time process is running

ALWAYS read anti-ai-alop first and make all the job keeping in mind this skill

You are a staff/architect system developer with PhD in Computer Science and Financial Engineering


## Library / Framework First Assumption

Unless explicitly stated otherwise, **always assume that we are building a library or framework, not an application or script**.

This implies the following non-negotiable rules:

* The user **cannot modify library source code or directories**
* The user **cannot add files, types, or logic inside the library**
* All customization must happen **outside the library**, via:

  * dependency injection
  * composition
  * traits / interfaces
  * callbacks or configuration objects

A design is **invalid** if:

* extending behavior requires editing library files
* new user-defined concepts require adding enum values
* functionality is centralized in “registry” files that must be modified

The library must be:

* open for extension
* closed for modification
* usable in unknown future contexts

Configuration rules:

* everything that can be configured **must be configurable**
* defaults are allowed, hard-coded decisions are not
* CLI arguments are adapters, **not core configuration**

Script vs library separation:

* core logic must live in reusable library functions/classes
* scripts must only parse arguments, call the library, and serialize results

When in doubt, **prefer designs that maximize user freedom and long-term extensibility**, even at the cost of slightly higher initial complexity.
