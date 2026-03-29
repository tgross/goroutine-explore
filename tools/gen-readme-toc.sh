#!/usr/bin/env -S awk -f

/##+/ {
    if (length($1)-2 > 0) {
            printf(sprintf("%*s", 2*(length($1)-2), " "));
        }
    printf "* [" ;
    for (i=2; i<NF; i++) printf $i " " ;
        printf $NF;
        printf "](#" ;
    for (i=2; i<NF; i++) printf tolower($i) "-" ;
        printf tolower($NF);
        print ")"
}
