#!/usr/bin/env bash

set -ve

# Requires the following command beforehand:
#   $ gem install travis
#   $ travis login --auto
#   $ travis encrypt-file ci/kube-aws-bot-git-ssh-key --repo <your github user or organization>/kube-aws

# And then change this line to the one output from the `travis encrypt-file` command above
openssl aes-256-cbc -K $encrypted_514cf8442810_key -iv $encrypted_514cf8442810_iv -in kube-aws-bot-git-ssh-key.enc -out ci/kube-aws-bot-git-ssh-key -d

# Prevent the following error:
#   Permissions 0644 for '/home/travis/gopath/src/github.com/kubernetes-incubator/kube-aws/ci/kube-aws-bot-git-ssh-key' are too open.
#   ...
#   bad permissions: ignore key: /home/travis/gopath/src/github.com/kubernetes-incubator/kube-aws/ci/kube-aws-bot-git-ssh-key
chmod 600 ci/kube-aws-bot-git-ssh-key

# Finally,
#   $ git add kube-aws-bot-git-ssh-key.enc
#   $   $ git commit -m '...'

echo -e "Host github.com\n\tStrictHostKeyChecking no\nIdentityFile $(pwd)/ci/kube-aws-bot-git-ssh-key\n" >> ~/.ssh/config

ssh git@github.com

if [ $? -ne 1 ]; then
  echo ssh connection check to github failed 1>&2
  exit 1
fi

make publish-docs
