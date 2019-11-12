ifmgrd design
=============

Overview
--------
The ifmgrd service provides a mechanism for applying interface
configuration at plug time. It handles interface config events for
registered interfaces. For this to work properly configd must be told
not to run the commit actions by using the
'configd:defer-child-actions' YANG statement in the desired
interface's data-model. These deferred actions will then be run
asynchronously by ifmgrd either immediately if the interface exists or
at plug time when the interface comes into existance. The ifmgrd
service also emulates the configd commit time interface so the
environment the deferred action scripts execute in look very similar
to the environment configd would run them in.

Each managed interface is controlled by a state-machine that tracks
the current state and transitions based on the received events. Events
are generated with the ifmgrctl utility.

**NOTE:** ifmgrd is meant to be a temporary solution providing hotplug
  configuration services using legacy commit scripts. A more permanent
  solution would not rely on the commit library to apply the
  configuration but would have a more direct mechanism, and an
  intermediate configuration file format. This services state-machine,
  would however provide an appropriate basis for such a daemon.


interface state-machine transition table
----------------------------------------

| state     | event    | action                                  | new state                                      |
|-----------|----------|-----------------------------------------|------------------------------------------------|
| unplugged | apply    | stage new config                        | unplugged                                      |
| unplugged | reset    | delete staged config                    | unplugged                                      |
| unplugged | plug     | apply staged config                     | applying                                       |
| unplugged | kill     | shutdown state-machine                  | shutdown                                       |
| applying  | apply    | stage new config                        | applying                                       |
| applying  | reset    | stage empty config                      | applying                                       |
| applying  | unplug   | remove running config                   | unplugged                                      |
| applying  | shutdown | shutdown state-machine                  | shutdown                                       |
| applying  | done     | set running = applied config            | if candidate != running  applying else plugged |
| plugged   | apply    | stage new config; apply staged config   | applying                                       |
| plugged   | reset    | stage empty config; apply staged config | applying                                       |
| plugged   | unplug   | remove running config                   | unplugged                                      |
| plugged   | kill     | shutdown state-machine                  | shutdown                                       |


ifmgrctl utility
----------------
```
Usage: ifmgrctl <action> <args>
Available actions:
  apply		apply latest config to managed interfaces
  plug		send plug event for device
  register	register a new device to be managed
  unplug	send unplug event for device
  unregister	stop managing a device

```

**Apply** downloads the latest configuration from configd and then sends
it to ifmgrd.

**Plug** signals that an interface was added to the system, if the
interface is not currently managed by ifmgrd the plug event is
ignored. This will apply the cached candidate configuration to the
interface.

**Unplug** signals that an interface was removed, this causes the running
configuration to be reset to an empty state. The candidate
configuration will remain and be applied on the next plug event.

**Register** signals to start listening for events on a given interface.

**Unregister** stops the state-machine for an interface and removes the
state from the manager. All previously applied configuration remains
active.
