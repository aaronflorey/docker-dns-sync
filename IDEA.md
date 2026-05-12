# IDEA

I run Godoxy on a couple machines, but having to manually manage the dns rewrites in Adguard Home is annoying.

ref: https://github.com/yusing/godoxy

Goal: A Go daemon that runs on your machine (or in docker), that connects to the docker dameon and watches for changes in labels that godoxy supports, then keep adguard in sync. i do feel like we need to keep state so we can remove domains that are no longer around even after a reboot.

godoxy is Go, so maybe we can import their label parsing stuff. because we need to support aliases etc. plus the no proxy label.

can you investigate how we could do this? i'd like live syncing so when i bring up a new server, i don't have to wait forever.

we may even just rely on the godoxy api otherwise.

i would like to expand this into other inputs/outputs in the future eg. nginx-proxy-manager/traefik/caddy for http, and then adguard-home, pihole, cloudlare etc. it'll need to be config driven (toml). 

so let's build it in a way that allows for multiple sources and multiple outputs. but we'll focus solely on adguard-home and godoxy for now.

can you please do some research into this, and then when you have enough idea on what is possible, start the `prd` skill to ask me questions to narrow down the PRD
