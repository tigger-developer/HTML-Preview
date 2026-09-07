#!/usr/bin/awk -f
# org-prepass.awk — rescue the two things pandoc's org reader throws away
# before any Lua filter can see them.
#
#   1. planning lines (DEADLINE:/SCHEDULED:/CLOSED:) -> #+begin_planning block
#   2. :LOGBOOK: drawers                             -> :LOGBOOK_: (a generic
#                                                       drawer, which survives)
#
# Usage:
#   awk -f org-prepass.awk doc.org | pandoc -f org -t html5 -s ...
#
# Both survive as <div class="planning"> and <div class="LOGBOOK_ drawer">,
# which org-fidelity.css styles.

/^[ \t]*:LOGBOOK:[ \t]*$/ {
    sub(/:LOGBOOK:/, ":LOGBOOK_:")
    print
    next
}

/^[ \t]*(DEADLINE|SCHEDULED|CLOSED):[ \t]*[<[]/ {
    print "#+begin_planning"
    print
    print "#+end_planning"
    next
}

{ print }
