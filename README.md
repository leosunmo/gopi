# Gopi

PyPi server in Go

## Dev
```
docker-compose up -d --build
```
Go to http://localhost:9000 and make sure you create a bucket called "gopi" or whatever you change it to the in `docker-compose.yaml` file.

Access Key and Secret Key is `minioadmin` by default.

A good test package to upload is https://github.com/prometheus/client_python.
```
mkdir tmp
cd tmp/
git clone https://github.com/prometheus/client_python.git

cd client_python
sudo python3 setup.py sdist upload -r http://localhost:8080/simple
```

Tailing the logs of `gopi` with `docker-compose logs -f gopi` will give you a lot of debug output since the `client_python` package passes it's markdown README in the `description` paramter in the POST form.

Hopefully you should see a new directory and files in Minio at http://localhost:9000/.

http://localhost:8080/simple/ should also give you a list of the uploaded packages.
