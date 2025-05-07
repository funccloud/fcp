load('ext://restart_process', 'docker_build_with_restart')
load('ext://cert_manager', 'deploy_cert_manager')

# Ignore changes in the bin directory
watch_settings(ignore=['bin'])

# --- Configuration ---
REGISTRY = 'ghcr.io/funccloud/fcp' # Or use read_json('tilt_config.json').get('registry', 'default_registry')
VERSION = 'latest' # Read version from env var, default to latest
APPS = ['manager']
DEFAULT_APP = 'manager' # App used for single-app commands if needed

# --- Helper Functions ---

# Function to generate manifests (assuming common for all apps)
def manifests():
    cmd = (
        'bin/controller-gen rbac:roleName=manager-role crd webhook ' +
        # Use single quotes outside, double inside
        'paths="./..." ' +
        'output:crd:artifacts:config=config/crd/bases;'
    )
    return cmd

# Function to generate code (assuming common for all apps)
def generate():
    cmd = (
        # Use double quotes inside
        'bin/controller-gen object:headerFile="hack/boilerplate.go.txt" ' +
        # Use double quotes inside
        'paths="./..."' # Removed trailing semicolon
    )
    return cmd

# Function to build a specific app binary
def build_binary_cmd(app):
    # Using .format() instead of f-string
    return 'CGO_ENABLED=0 GOOS=linux go build -o ./bin/{} cmd/{}/main.go'.format(app, app)


# Function to generate k8s YAML after editing images for ALL apps
# ASSUMPTION: All apps are deployed via the kustomization in config/default
# ASSUMPTION: The image name to replace in kustomization is the app name itself (e.g., manager, fcp)
def generate_k8s_yaml_for_all_apps():
    kustomize_dir = 'config/default' # Always use config/default
    kustomize_bin = './bin/kustomize'
    edit_commands = []

    print('Preparing kustomize edit commands for all apps in {} (version: {})'.format(kustomize_dir, VERSION))
    for app in APPS:
        img = '{}/{}:{}'.format(REGISTRY, app, VERSION)
        # Use the app name directly as the image name in kustomization
        kustomize_image_name = app
        # Use string concatenation instead of .format()
        edit_cmd = kustomize_bin + ' edit set image ' + kustomize_image_name + '=' + img
        edit_commands.append(edit_cmd)
        print('  - Will set image for {} to {}'.format(app, img))

    # Combine all edit commands, change directory, run edits, change back, then build
    full_edit_cmd_str = "cd {}; {}; cd ../..".format(kustomize_dir, ' ; '.join(edit_commands))
    build_cmd = '{} build {}'.format(kustomize_bin, kustomize_dir)

    print('Running kustomize edits and build for {}'.format(kustomize_dir))
    # Execute edits first, then build
    local(full_edit_cmd_str)
    data = local(build_cmd)

    # Process the YAML (e.g., remove securityContext)
    decoded = decode_yaml_stream(data)
    if decoded:
        for d in decoded:
            if d.get("kind") == "Deployment":
                spec = d.get('spec', {}).get('template', {}).get('spec', {})
                if "securityContext" in spec:
                    spec.pop('securityContext')
                containers = spec.get('containers', [])
                for c in containers:
                    if "securityContext" in c:
                        c.pop('securityContext')
    return encode_yaml_stream(decoded)

# --- Setup ---
# Ensure tools are present (controller-gen, kustomize)
local('make controller-gen kustomize')
# Initial generation of CRDs/RBAC/webhooks and Go code
local(manifests() + generate())

# --- Deploy Cert Manager ---
deploy_cert_manager(version='v1.17.1') # Use appropriate version

# --- CRDs (Assuming common for all apps) ---
# Apply CRDs first
local_resource('crd', manifests() + 'bin/kustomize build config/crd | kubectl apply -f -', deps=['api'])
# Resource to manually remove CRDs if needed
local_resource('un-crd', 'bin/kustomize build config/crd | kubectl delete -f -', auto_init=False, trigger_mode=TRIGGER_MODE_MANUAL)


# --- Build and Live Update Each App (Docker Build part) ---
app_images = [] # Collect image names for dependency
for app in APPS:
    app_img = '{}/{}:{}'.format(REGISTRY, app, VERSION) # Use VERSION
    app_images.append(app_img) # Add image to list for later dependency
    app_binary_path = './bin/{}'.format(app)
    app_cmd_path = 'cmd/{}'.format(app)
    app_entrypoint = '/{}'.format(app) # Entrypoint inside the container

    # 1. Local resource to compile the Go binary for the app
    local_resource(
        'compile_{}'.format(app),
        cmd=build_binary_cmd(app),
        deps=['internal', app_cmd_path, 'go.mod', 'go.sum', 'api']
    )

    # 2. Docker build for the app using the specific binary
    docker_build_with_restart(
        app_img, '.', 
        dockerfile='tilt.docker',
        build_args={'APP': app},
        entrypoint=app_entrypoint,
        only=[app_binary_path],
        trigger='compile_{}'.format(app),
        live_update=[
            sync(app_binary_path, app_entrypoint),
        ]
    )

# --- Deploy K8s Resources (after all images are built) ---
# This single call generates YAML after editing images for ALL apps
# It depends on all the application images being successfully built
k8s_yaml(generate_k8s_yaml_for_all_apps())


print("Tiltfile configured for apps: {} with registry {} and version {}".format(APPS, REGISTRY, VERSION))