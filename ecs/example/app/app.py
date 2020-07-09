
from flask import Flask
from flask import render_template
from redis import StrictRedis
from datetime import datetime

app = Flask(__name__)
redis = StrictRedis(host='backend', port=6379)


@app.route('/')
def home():
    redis.lpush('times', datetime.now().strftime('%Y-%m-%dT%H:%M:%S%z'))
    return render_template('index.html', title='Home',
                           times=redis.lrange('times', 0, -1))


if __name__ == '__main__':
    app.run(host='0.0.0.0', debug=True)
