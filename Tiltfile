load('ext://restart_process', 'docker_build_with_restart')
load('ext://cert_manager', 'deploy_cert_manager')


IMG = 'controller:latest'
#docker_build(IMG, '.')

def yaml():
    return local('cd config/manager; kustomize edit set image controller=' + IMG + '; cd ../..; kustomize build config/default')

def manifests():
    return 'controller-gen crd:trivialVersions=true rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases;'

def generate():
    return 'controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./...";'

def vetfmt():
    return 'go vet ./...; go fmt ./...'

def binary():
    return 'CGO_ENABLED=0 GOOS=linux go build -o manager cmd/main.go'

local(manifests() + generate())

deploy_cert_manager(version='v1.17.1')

local_resource('crd', manifests() + 'kustomize build config/crd | kubectl apply -f -', deps=['api', 'internal'])

local_resource('un-crd', 'kustomize build config/crd | kubectl delete -f -', auto_init=False, trigger_mode=TRIGGER_MODE_MANUAL)

k8s_yaml(yaml())

local_resource('recompile', generate() + binary(), deps=['internal', 'cmd'])

docker_build_with_restart(IMG, '.', 
 dockerfile='tilt.docker',
 entrypoint='/manager',
 only=['manager'],
 live_update=[
       sync('manager', '/manager'),
   ]
)