Make reboot + poweroff work on Void/SSH:

```
root@template-vm ~]# cat /etc/bash/bashrc.d/reboot.sh
alias reboot='shutdown -r now && exit'

[root@template-vm ~]# cat /etc/bash/bashrc.d/poweroff.sh
alias poweroff='shutdown -h now && exit'
```
