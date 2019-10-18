set -e

dir=/var/lib/direktil/
PATH=$PATH:$dir
cd $dir

if [ ! -f govc.env ]; then
    echo ERROR: govc.env file not found in dir $dir ; exit 1
fi
source govc.env

if [ $# != 2 ]; then
    echo "Usage: $0 <VM_NAME> <NOVIT_HOST>" ; exit 1
fi

if [[ -z $NOVIT_VM_FOLDER || -z $NOVIT_ISO_FOLDER ]]; then
    echo "ERROR: All GOVC env vars (including NOVIT_VM_FOLDER and NOVIT_ISO_FOLDER) must be provided" ; exit 1
fi

VM=$1
HOST=$2

govc vm.power -off $NOVIT_VM_FOLDER/$VM || true
sleep 5
curl localhost:7606/hosts/$HOST/boot.iso | govc datastore.upload - $NOVIT_ISO_FOLDER/$VM.iso
sleep 5
govc vm.power -on $NOVIT_VM_FOLDER/$VM

