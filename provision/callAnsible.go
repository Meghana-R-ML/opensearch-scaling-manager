package provision

import (
	"context"

	"github.com/apenella/go-ansible/pkg/execute"
	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	"github.com/apenella/go-ansible/pkg/stdoutcallback/results"
)

func CallScaleUp(username string, hosts string) {

	ansiblePlaybookConnectionOptions := &options.AnsibleConnectionOptions{
		User: username,
	}

	ansiblePlaybookOptions := &playbook.AnsiblePlaybookOptions{
		Inventory: hosts,
		ExtraVarsFile: []string{"@provision/ansible_scripts/varsFile.yml"},
	}

	ansiblePlaybookPrivilegeEscalationOptions := &options.AnsiblePrivilegeEscalationOptions{
		Become:        true,
	}

	playbook := &playbook.AnsiblePlaybookCmd{
		Playbooks:                  []string{"provision/ansible_scripts/scaleUpPlaybook.yml"},
		ConnectionOptions:          ansiblePlaybookConnectionOptions,
		PrivilegeEscalationOptions: ansiblePlaybookPrivilegeEscalationOptions,
		Options:                    ansiblePlaybookOptions,
		Exec: execute.NewDefaultExecute(
			execute.WithEnvVar("ANSIBLE_FORCE_COLOR", "true"),
			execute.WithTransformers(
				results.Prepend("Go-ansible with become"),
			),
		),
	}

	err := playbook.Run(context.TODO())
	if err != nil {
		panic(err)
	}
}

func CallScaleDown(username string, hosts string) {

        ansiblePlaybookConnectionOptions := &options.AnsibleConnectionOptions{
                User: username,
        }

        ansiblePlaybookOptions := &playbook.AnsiblePlaybookOptions{
                Inventory: hosts,
		ExtraVarsFile: []string{"@provision/ansible_scripts/varsFile.yml"},
        }

        ansiblePlaybookPrivilegeEscalationOptions := &options.AnsiblePrivilegeEscalationOptions{
                Become:        true,
        }

        playbook := &playbook.AnsiblePlaybookCmd{
                Playbooks:                  []string{"provision/ansible_scripts/scaleDownPlaybook.yml"},
                ConnectionOptions:          ansiblePlaybookConnectionOptions,
                PrivilegeEscalationOptions: ansiblePlaybookPrivilegeEscalationOptions,
                Options:                    ansiblePlaybookOptions,
                Exec: execute.NewDefaultExecute(
                        execute.WithEnvVar("ANSIBLE_FORCE_COLOR", "true"),
                        execute.WithTransformers(
                                results.Prepend("Go-ansible with become"),
                        ),
                ),
        }

        err := playbook.Run(context.TODO())
        if err != nil {
                panic(err)
        }
}

