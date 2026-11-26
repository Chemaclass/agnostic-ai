import unittest

from greeting import greet


class GreetingTest(unittest.TestCase):
    def test_greets_a_named_person(self):
        self.assertEqual(greet("Ada"), "Hello, Ada!")
