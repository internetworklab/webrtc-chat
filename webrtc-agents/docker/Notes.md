This folder put docker-compose.yaml files base on variants (i.e. the use case)

For DN42 scenario, it is a bit more complicated because you have to setup container networking in order to make it work (that's what `dn42/init.d/*.sh` and `dn42/deinit.d/*.sh` is for) .

For Internet/clearnet use, it is much simpler.
