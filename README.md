# Reference Cloud infrastructure (IaC)

This repository declares infrastructure of Gigit cloud as a code using [Terraform](https://www.terraform.io/).

## Dependencies

- Terraform v1.2.6
- AWS credentials for accessing Terraform state (hosted in S3 bucket)
- gomplate, use your local dependency management system for it, for mac: `brew install gomplate`
- GNU Make (should be part of any system by default). Optional, you can run command from makefile directly in terminal.

1. Copy everything from `project_reference` folder to your local repo. Those content is your project specific data and depends on your project only. All other data is updatable and a subject to be changed.
   
2. Copy `architecture` repo to `architecture` subfolder of your project. You can just copy it, or you can make a [git submodule](https://git-scm.com/book/en/v2/Git-Tools-Submodules)

The final structure should looks like that:
```
./Makefile
./dev.yaml
./prod.yaml
./architecture/ <- this folder could be replaced by new updated version 
    ./docs
    ./env
    ....
```

3. Edit `dev.yaml` file and run make comand:

```sh
    make dev
```

or 

```sh
    gomplate -c vars=dev.yaml -f ./architecture/env/main.tmpl   -o ./architecture/env/dev/main.tf
```

4. Init Terraform:

```sh
    cd env/dev
    terraform init
```

5. Run and save terraform plan:

```sh
    terraform plan
```

6. Apply when you're happy with the plan:

```sh
    terraform apply
```

## Architecture

![Architecture diagram](./docs/images/architecture.png)


## Health check

All services by default should respond status `200` on GET handler with path `/health/love`. If it is not responding with status 200, the application load balancer will consider the service unhealthy and redeploy it. 


## Remote dubug

[You can use Amazon ECS Exec](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-exec.html) to  execute command remotely in terminal.

To do so, you need to install [AWS Session Management Plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html#install-plugin-macos) on your machine.

For mac Mx you  need:

```shell
curl "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/mac_arm64/session-manager-plugin.pkg" -o "session-manager-plugin.pkg"
sudo installer -pkg session-manager-plugin.pkg -target /
sudo ln -s /usr/local/sessionmanagerplugin/bin/session-manager-plugin /usr/local/bin/session-manager-plugin

```

After that you can verify the installation: `session-manager-plugin`.

With session manager you can login to container, execut a command in container or do a port forwarding.

You can use a [usefull script](https://github.com/aws-containers/amazon-ecs-exec-checker) to help you work with AWS Exec.



