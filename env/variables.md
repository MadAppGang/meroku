
## Variables
    
1. Main settings
    1. projectname !
    2. environment | dev
    3. region | us-east-1 
    4. state_bucket | projectname-terraform-state-dev-[random]
    5. state_file | state.tfstate
1. Backend
    1. slack_deployment_webhook | 
    1. backend_bucket_postfix |
    1. backend_bucket_public | true
    1. health_endpoint | /health/live
    1. docker_image |
    1. setup_FCM_SNS | false
    1. backend_image_port | 8080
    1. github_oidc_enabled | false
    1. github_subjects | [repo:MadAppGang/*]
    1. backend_container_command | [config, value]
    1. pgadmin_enabled | false
    1. pgadmin_email | admin@admin.com
    1. xray_enabled | false
    1. backend_env | [{ "name" : UPPERCASE("BACKEND_TEST"), "value" : "TEST" },]
1. Domain
    1. setup_domain > yes/no/use_existing
    2. domain_name
1. Postgres
    1. setup_postgres > yes/> [!NOTE]
    1. db_name | projectname-env
    1. username | projectname
    1. public_access | true
    1. engine_version | 14:> [!WARNING]
1. Cognito
    1. setup_cognito > yes/> [!NOTE]
    1.enable_web_client > yes/> [!NOTE]
    1. enable_dashboard_client > yes/> [!NOTE]
    1. dashboard_callback_urls | [https://jwt.io]
    1. enable_user_pool_domain > yes/> [!NOTE]
    1. user_pool_domain_prefix |
    1. allow_backend_task_to_confirm_signup > yes/> [!NOTE]
    1. auto_verified_attributes | [email]Main
1. Scheduled tasks (array of items)
    1. name | task1
    1. schedule | rate(1 day)    
1. Event processing tasks (array of items)
    1. name | event_task1
    1. rule_name | hug_all
    1. detail_types |
    1. sources | [service1, service2]
1. SES
    1. setup_ses > yes/> [!NOTE]
    1. ses_domain | email.dev.madappgang.com
    1. ses_test_emails | [i@madappgang.com, ivan.holiak@madappgang.com 
