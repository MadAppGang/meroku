import boto3
import json
import typer

def savessm(param_name: str):
    session = boto3.Session()
    client = session.client('ssm')
    parameter_value = client.get_parameter(Name=param_name)['ParameterValue']
    vars = json.loads(parameter_value)
    with open(".env", "w") as f:
        for key, value in vars.items():
            f.write(f"{key}={value}\n")    

if __name__ == "__main__":
    typer.run(savessm)

